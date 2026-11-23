package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/sound"
)

func (s *Store) EnsureBuiltinSounds(ctx context.Context, sounds []sound.Sound) error {
	now := time.Now().UnixMilli()
	for _, value := range sounds {
		_, err := s.db.ExecContext(ctx, `INSERT INTO sounds(id,key,name,category,sort_order,bus,volume,loop,fade_in_ms,fade_out_ms,builtin_path,builtin,enabled,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(key) DO UPDATE SET builtin_path=excluded.builtin_path,builtin=1`,
			value.ID, value.Key, value.Name, value.Category, value.SortOrder, value.Bus, value.Volume, value.Loop,
			value.FadeInMS, value.FadeOutMS, value.BuiltinPath, true, value.Enabled, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListSounds(ctx context.Context, includeDisabled bool) ([]sound.Sound, error) {
	query := `SELECT id,key,name,category,sort_order,bus,volume,loop,fade_in_ms,fade_out_ms,
		COALESCE(builtin_path,''),COALESCE(asset_id,''),builtin,enabled,created_at,updated_at
		FROM sounds WHERE archived_at IS NULL`
	if !includeDisabled {
		query += " AND enabled=1"
	}
	query += " ORDER BY category,sort_order,name"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]sound.Sound, 0)
	for rows.Next() {
		value, err := scanSound(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) GetSound(ctx context.Context, id string) (sound.Sound, error) {
	return scanSound(s.db.QueryRowContext(ctx, `SELECT id,key,name,category,sort_order,bus,volume,loop,fade_in_ms,fade_out_ms,
		COALESCE(builtin_path,''),COALESCE(asset_id,''),builtin,enabled,created_at,updated_at
		FROM sounds WHERE id=? AND archived_at IS NULL`, id))
}

func (s *Store) CreateSound(ctx context.Context, value sound.Sound, actor game.Actor) (sound.Sound, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sound.Sound{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sounds(id,key,name,category,sort_order,bus,volume,loop,fade_in_ms,fade_out_ms,asset_id,builtin,enabled,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.Key, value.Name, value.Category, value.SortOrder,
		value.Bus, value.Volume, value.Loop, value.FadeInMS, value.FadeOutMS, value.AssetID, false, value.Enabled, now, now); err != nil {
		return sound.Sound{}, err
	}
	if err := insertAudit(ctx, tx, actor, "sound.create", "sound", value.ID, "", nil, value, ""); err != nil {
		return sound.Sound{}, err
	}
	if err := tx.Commit(); err != nil {
		return sound.Sound{}, err
	}
	return s.GetSound(ctx, value.ID)
}

func (s *Store) UpdateSound(ctx context.Context, id string, update sound.Update, actor game.Actor) (sound.Sound, error) {
	before, err := s.GetSound(ctx, id)
	if err != nil {
		return sound.Sound{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sound.Sound{}, err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(ctx, `UPDATE sounds SET name=?,category=?,sort_order=?,bus=?,volume=?,loop=?,fade_in_ms=?,fade_out_ms=?,enabled=?,updated_at=? WHERE id=? AND archived_at IS NULL`,
		update.Name, update.Category, update.SortOrder, update.Bus, update.Volume, update.Loop, update.FadeInMS, update.FadeOutMS, update.Enabled, now, id)
	if err != nil {
		return sound.Sound{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return sound.Sound{}, sql.ErrNoRows
	}
	after := before
	after.Name, after.Category, after.SortOrder = update.Name, update.Category, update.SortOrder
	after.Bus, after.Volume, after.Loop = update.Bus, update.Volume, update.Loop
	after.FadeInMS, after.FadeOutMS, after.Enabled = update.FadeInMS, update.FadeOutMS, update.Enabled
	after.UpdatedAt = time.UnixMilli(now)
	if err := insertAudit(ctx, tx, actor, "sound.update", "sound", id, "", before, after, ""); err != nil {
		return sound.Sound{}, err
	}
	if err := tx.Commit(); err != nil {
		return sound.Sound{}, err
	}
	return s.GetSound(ctx, id)
}

func (s *Store) ArchiveSound(ctx context.Context, id string, actor game.Actor) error {
	before, err := s.GetSound(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(ctx, `UPDATE sounds SET archived_at=?,updated_at=? WHERE id=? AND builtin=0 AND archived_at IS NULL`, now, now, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return sql.ErrNoRows
	}
	if err := insertAudit(ctx, tx, actor, "sound.archive", "sound", id, "", before, map[string]any{"archived": true}, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func scanSound(row scanner) (sound.Sound, error) {
	var value sound.Sound
	var created, updated int64
	err := row.Scan(&value.ID, &value.Key, &value.Name, &value.Category, &value.SortOrder, &value.Bus,
		&value.Volume, &value.Loop, &value.FadeInMS, &value.FadeOutMS, &value.BuiltinPath, &value.AssetID,
		&value.Builtin, &value.Enabled, &created, &updated)
	value.CreatedAt = time.UnixMilli(created)
	value.UpdatedAt = time.UnixMilli(updated)
	return value, err
}
