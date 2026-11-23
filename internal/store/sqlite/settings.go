package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/onlyin32bit/4t/internal/settings"
)

func (s *Store) LoadSettings(ctx context.Context) (settings.Values, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT values_json FROM site_settings WHERE id = 1`).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return settings.Values{}, settings.ErrNotFound
		}
		return settings.Values{}, err
	}
	var values settings.Values
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return settings.Values{}, err
	}
	return values, nil
}

func (s *Store) SaveSettings(ctx context.Context, values settings.Values) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO site_settings(id, values_json, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET values_json = excluded.values_json, updated_at = excluded.updated_at`, string(raw), time.Now().UnixMilli())
	return err
}

func (s *Store) SaveSettingsAndAudit(ctx context.Context, values settings.Values, audit settings.AuditEntry) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx, `INSERT INTO site_settings(id, values_json, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET values_json = excluded.values_json, updated_at = excluded.updated_at`, string(raw), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,match_id,before_json,after_json,reason,client_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		nullInt(audit.ActorID), audit.Action, audit.EntityType, audit.EntityID, nil, string(audit.Before), string(audit.After), audit.Reason, audit.ClientID, now); err != nil {
		return err
	}
	return tx.Commit()
}
