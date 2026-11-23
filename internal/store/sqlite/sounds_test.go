package sqlite

import (
	"context"
	"database/sql"
	"io/fs"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

func TestFreshDatabaseSeedsSoundBuses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureBuiltinSounds(ctx, sound.Builtins()); err != nil {
		t.Fatal(err)
	}
	assertBuiltinMix(t, store)
}

// An existing pre-008 database (music/effects only) is reclassified.
func TestMigrationReclassifiesExistingBuiltins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "4t.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"001_initial.sql", "002_matches.sql", "003_remove_match_number.sql", "004_session_management.sql", "005_sounds.sql", "006_settings.sql", "007_system_settings.sql"} {
		contents, err := fs.ReadFile(migrations, "migrations/"+name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, int(name[2]-'0'), time.Now().Unix()); err != nil {
			t.Fatal(err)
		}
	}
	// Old seeding: every *Seconds/StartTurn/... sound on music, all at volume 1.
	now := time.Now().UnixMilli()
	for _, value := range sound.Builtins() {
		if _, err := db.ExecContext(ctx, `INSERT INTO sounds(id,key,name,category,sort_order,bus,volume,loop,fade_in_ms,fade_out_ms,builtin_path,builtin,enabled,created_at,updated_at)
			VALUES(?,?,?,?,?,'music',1,0,0,120,?,1,1,?,?)`, value.ID, value.Key, value.Name, value.Category, value.SortOrder, value.BuiltinPath, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureBuiltinSounds(ctx, sound.Builtins()); err != nil {
		t.Fatal(err)
	}
	assertBuiltinMix(t, store)
	// The relaxed CHECK constraint accepts the timer bus for edits.
	if _, err := store.UpdateSound(ctx, "builtin.main.start", sound.Update{Name: "Start", Category: "main", Bus: sound.BusTimer, Volume: 0.5, Enabled: true}, game.Actor{}); err != nil {
		t.Fatal(err)
	}
}

func assertBuiltinMix(t *testing.T, store *Store) {
	t.Helper()
	values, err := store.ListSounds(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]sound.Sound{}
	for _, value := range sound.Builtins() {
		want[value.ID] = value
	}
	if len(values) != len(want) {
		t.Fatalf("sounds = %d, want %d", len(values), len(want))
	}
	for _, value := range values {
		expected := want[value.ID]
		if value.Bus != expected.Bus || value.Loop != expected.Loop || math.Abs(value.Volume-expected.Volume) > 1e-9 {
			t.Fatalf("%s: got bus=%s loop=%v volume=%v, want bus=%s loop=%v volume=%v", value.ID,
				value.Bus, value.Loop, value.Volume, expected.Bus, expected.Loop, expected.Volume)
		}
	}
}

func migrationFiles() (int, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			count++
		}
	}
	return count, nil
}
