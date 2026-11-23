package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/onlyin32bit/4t/internal/config"
)

func (s *Store) LoadSystemSettings(ctx context.Context) (config.SystemSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT values_json FROM system_settings WHERE id = 1`).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return config.SystemSettings{}, config.ErrNotFound
		}
		return config.SystemSettings{}, err
	}
	var values config.SystemSettings
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return config.SystemSettings{}, err
	}
	return values, nil
}

func (s *Store) SaveSystemSettings(ctx context.Context, values config.SystemSettings) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO system_settings(id, values_json, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET values_json = excluded.values_json, updated_at = excluded.updated_at`, string(raw), time.Now().UnixMilli())
	return err
}
