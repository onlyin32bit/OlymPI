package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("restrict data directory: %w", err)
	}

	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := restrictDatabaseFiles(abs); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func restrictDatabaseFiles(path string) error {
	for _, value := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(value, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("restrict database file %q: %w", value, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return fmt.Errorf("invalid migration name %q", entry.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("invalid migration version %q: %w", entry.Name(), err)
		}

		var applied int
		err = s.db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&applied)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check migration %d: %w", version, err)
		}

		contents, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %d: %w", version, err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)", version, time.Now().Unix()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}
	}
	return nil
}

func (s *Store) HasAdmin(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE role = ?)", auth.RoleAdmin).Scan(&exists)
	return exists, err
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, role auth.Role) (auth.User, error) {
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO users(username, password_hash, role) VALUES (?, ?, ?)", username, passwordHash, role)
	if err != nil {
		return auth.User{}, fmt.Errorf("create user: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return auth.User{}, fmt.Errorf("read user id: %w", err)
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (auth.User, error) {
	return s.getUser(ctx, "SELECT id, username, password_hash, role, created_at, COALESCE(fixed_slot, 0), password_change_required, disabled FROM users WHERE id = ?", id)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (auth.User, error) {
	return s.getUser(ctx, "SELECT id, username, password_hash, role, created_at, COALESCE(fixed_slot, 0), password_change_required, disabled FROM users WHERE username = ?", username)
}

func (s *Store) getUser(ctx context.Context, query string, arg any) (auth.User, error) {
	var u auth.User
	var createdAt int64
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt, &u.FixedSlot, &u.PasswordChangeRequired, &u.Disabled)
	if err != nil {
		return auth.User{}, err
	}
	u.CreatedAt = time.Unix(createdAt, 0)
	return u, nil
}

func (s *Store) CreateFixedContestant(ctx context.Context, slot int, username, passwordHash string) (auth.User, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO users(username, password_hash, role, fixed_slot, password_change_required)
		VALUES (?, ?, ?, ?, 1)`, username, passwordHash, auth.RoleContestant, slot)
	if err != nil {
		return auth.User{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return auth.User{}, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) ListFixedContestants(ctx context.Context) ([]auth.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, password_hash, role, created_at,
		COALESCE(fixed_slot, 0), password_change_required, disabled FROM users WHERE fixed_slot IS NOT NULL ORDER BY fixed_slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]auth.User, 0)
	for rows.Next() {
		var user auth.User
		var createdAt int64
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &createdAt, &user.FixedSlot, &user.PasswordChangeRequired, &user.Disabled); err != nil {
			return nil, err
		}
		user.CreatedAt = time.Unix(createdAt, 0)
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateFixedContestantPassword(ctx context.Context, slot int, hash string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, password_change_required = 0 WHERE fixed_slot = ?`, hash, slot)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteSessionsByUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, id, ipAddress, userAgent string, expiresAt time.Time) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id, user_id, expires_at, created_at, last_seen_at, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, userID, expiresAt.Unix(), now, now, ipAddress, userAgent)
	return err
}

func (s *Store) ReplaceSession(ctx context.Context, userID int64, id, ipAddress, userAgent string, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id, user_id, expires_at, created_at, last_seen_at, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, userID, expiresAt.Unix(), now, now, ipAddress, userAgent); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetSession(ctx context.Context, id string) (int64, time.Time, error) {
	var userID, expiresAt int64
	err := s.db.QueryRowContext(ctx, "SELECT user_id, expires_at FROM sessions WHERE id = ?", id).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, time.Time{}, err
	}
	return userID, time.Unix(expiresAt, 0), nil
}

func (s *Store) UpdateSessionExpiration(ctx context.Context, id string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET expires_at = ? WHERE id = ?", expiresAt.Unix(), id)
	return err
}

func (s *Store) TouchSession(ctx context.Context, id string, seenAt time.Time) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET last_seen_at = ? WHERE id = ? AND last_seen_at < ?", seenAt.Unix(), id, seenAt.Add(-15*time.Second).Unix())
	return err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)
	return err
}

func (s *Store) ListSessions(ctx context.Context) ([]auth.Session, error) {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", now); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sessions.id, users.id, users.username, users.role,
		COALESCE(users.fixed_slot, 0), sessions.ip_address, sessions.user_agent, sessions.created_at,
		sessions.last_seen_at, sessions.expires_at
		FROM sessions JOIN users ON users.id = sessions.user_id
		ORDER BY sessions.last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]auth.Session, 0)
	for rows.Next() {
		var value auth.Session
		var createdAt, lastSeenAt, expiresAt int64
		if err := rows.Scan(&value.ID, &value.UserID, &value.Username, &value.Role, &value.FixedSlot,
			&value.IPAddress, &value.UserAgent, &createdAt, &lastSeenAt, &expiresAt); err != nil {
			return nil, err
		}
		value.CreatedAt = time.Unix(createdAt, 0)
		value.LastSeen = time.Unix(lastSeenAt, 0)
		value.ExpiresAt = time.Unix(expiresAt, 0)
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ListAdmins(ctx context.Context) ([]auth.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, password_hash, role, created_at,
		COALESCE(fixed_slot, 0), password_change_required, disabled FROM users WHERE role = ? ORDER BY username`, auth.RoleAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]auth.User, 0)
	for rows.Next() {
		var user auth.User
		var createdAt int64
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &createdAt,
			&user.FixedSlot, &user.PasswordChangeRequired, &user.Disabled); err != nil {
			return nil, err
		}
		user.CreatedAt = time.Unix(createdAt, 0)
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUserPassword(ctx context.Context, id int64, hash string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateUserPasswordAndDeleteSessions(ctx context.Context, id int64, hash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetUserDisabled(ctx context.Context, id int64, disabled bool) error {
	query := "UPDATE users SET disabled = ? WHERE id = ?"
	args := []any{disabled, id}
	if disabled {
		// Keep the last-enabled-admin check in the same write statement as the
		// disable. SQLite serializes writers, so concurrent requests cannot
		// both observe the same enabled-admin count and disable the last two
		// administrators.
		query = `UPDATE users SET disabled = ? WHERE id = ?
			AND (role <> ? OR disabled = 1 OR (
				SELECT COUNT(*) FROM users WHERE role = ? AND disabled = 0
			) > 1)`
		args = []any{disabled, id, auth.RoleAdmin, auth.RoleAdmin}
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 1 {
		return nil
	}
	if disabled {
		var role auth.Role
		err := s.db.QueryRowContext(ctx, "SELECT role FROM users WHERE id = ?", id).Scan(&role)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sql.ErrNoRows
			}
			return err
		}
		if role == auth.RoleAdmin {
			return errors.New("at least one administrator must remain enabled")
		}
	}
	return sql.ErrNoRows
}

func (s *Store) CountEnabledAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE role = ? AND disabled = 0", auth.RoleAdmin).Scan(&count)
	return count, err
}
