package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/settings"
	"golang.org/x/crypto/bcrypt"
)

func TestSaveSettingsAndAuditCommitsAtomically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	values := settings.Defaults()
	values.EventName = "Atomic event"
	before := []byte(`{"event_name":"old"}`)
	after := []byte(`{"event_name":"Atomic event"}`)
	if err := store.SaveSettingsAndAudit(ctx, values, settings.AuditEntry{
		Action: "settings.update", EntityType: "settings", EntityID: "global",
		Before: before, After: after, ClientID: "test-client",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != values {
		t.Fatalf("settings mismatch: got %+v want %+v", got, values)
	}
	audits, err := store.ListAudit(ctx, "settings.update", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || string(audits[0].After) != string(after) {
		t.Fatalf("expected matching settings audit, got %+v", audits)
	}
}

func TestConcurrentAdminDisablesKeepOneEnabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := store.CreateUser(ctx, "first", "hash", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateUser(ctx, "second", "hash", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []int64{first.ID, second.ID} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			results <- store.SetUserDisabled(ctx, id, true)
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)

	var failure error
	for err := range results {
		if err != nil {
			failure = err
		}
	}
	if failure == nil {
		t.Fatal("expected one concurrent disable to be rejected")
	}
	if failure.Error() != "at least one administrator must remain enabled" {
		t.Fatalf("unexpected concurrent disable error: %v", failure)
	}

	enabled, err := store.CountEnabledAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatalf("expected one enabled administrator, got %d", enabled)
	}
}

func TestOpenMigratesFreshDatabaseAndPersistsUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "4t.db")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(store)
	if _, err := service.CreateAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hasAdmin, err := store.HasAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAdmin {
		t.Fatal("expected administrator to survive reopening the database")
	}

	var migrations int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if migrations != entries {
		t.Fatalf("expected %d applied migrations, got %d", entries, migrations)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	user, err := store.CreateUser(ctx, "operator", "hash", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := store.CreateSession(ctx, user.ID, "session", "127.0.0.1", "test", expiresAt); err != nil {
		t.Fatal(err)
	}
	userID, actualExpiry, err := store.GetSession(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	if userID != user.ID || !actualExpiry.Equal(expiresAt) {
		t.Fatalf("unexpected session: user=%d expiry=%s", userID, actualExpiry)
	}
}

func TestResetAdminPasswordRollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	oldHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, "operator", string(oldHash), auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := store.CreateSession(ctx, user.ID, "old-session", "127.0.0.1", "test", expiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER block_session_deletes
		BEFORE DELETE ON sessions
		BEGIN
			SELECT RAISE(ABORT, 'session deletion blocked');
		END`); err != nil {
		t.Fatal(err)
	}

	err = auth.NewService(store).ResetAdminPassword(ctx, user.ID, "new-password")
	if err == nil || !strings.Contains(err.Error(), "session deletion blocked") {
		t.Fatalf("expected session revocation failure, got %v", err)
	}

	updated, err := store.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("old-password")); err != nil {
		t.Fatalf("password changed despite failed session revocation: %v", err)
	}
	if _, _, err := store.GetSession(ctx, "old-session"); err != nil {
		t.Fatalf("old session was revoked despite failed reset: %v", err)
	}
}

func TestOpenRestrictsDatabasePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "4t.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("expected data directory mode 0700, got %04o", got)
	}
	dbInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := dbInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected database mode 0600, got %04o", got)
	}
}
