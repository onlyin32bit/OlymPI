package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/onlyin32bit/4t/internal/game"
)

func (s *Store) ListQuestionSets(ctx context.Context, search string) ([]game.QuestionSet, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT q.id,q.name,q.category,COALESCE(q.current_revision_id,''),
		COALESCE(r.revision,0),q.version,q.created_at,q.updated_at,q.archived_at,
		(SELECT COUNT(*) FROM matches m WHERE m.question_set_id=q.id)
		FROM question_sets q LEFT JOIN question_set_revisions r ON r.id=q.current_revision_id
		WHERE q.name LIKE ? OR q.category LIKE ? ORDER BY q.updated_at DESC`, "%"+search+"%", "%"+search+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]game.QuestionSet, 0)
	for rows.Next() {
		set, err := scanQuestionSet(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, set)
	}
	return result, rows.Err()
}

func scanQuestionSet(row scanner) (game.QuestionSet, error) {
	var set game.QuestionSet
	var created, updated int64
	var archived sql.NullInt64
	err := row.Scan(&set.ID, &set.Name, &set.Category, &set.CurrentRevisionID, &set.Revision, &set.Version, &created, &updated, &archived, &set.LinkedMatches)
	if err != nil {
		return game.QuestionSet{}, err
	}
	set.CreatedAt = time.UnixMilli(created)
	set.UpdatedAt = time.UnixMilli(updated)
	set.ArchivedAt = nullableTime(archived)
	return set, nil
}

func (s *Store) GetQuestionSet(ctx context.Context, id string) (game.QuestionSet, game.QuestionSetRevision, error) {
	row := s.db.QueryRowContext(ctx, `SELECT q.id,q.name,q.category,COALESCE(q.current_revision_id,''),COALESCE(r.revision,0),q.version,q.created_at,q.updated_at,q.archived_at,(SELECT COUNT(*) FROM matches m WHERE m.question_set_id=q.id) FROM question_sets q LEFT JOIN question_set_revisions r ON r.id=q.current_revision_id WHERE q.id=?`, id)
	set, err := scanQuestionSet(row)
	if err != nil {
		return game.QuestionSet{}, game.QuestionSetRevision{}, err
	}
	revision, err := s.getRevision(ctx, set.CurrentRevisionID)
	return set, revision, err
}

func (s *Store) getRevision(ctx context.Context, id string) (game.QuestionSetRevision, error) {
	var revision game.QuestionSetRevision
	var dataJSON string
	var created int64
	var actor sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id,question_set_id,revision,data_json,created_by,created_at FROM question_set_revisions WHERE id=?`, id).Scan(&revision.ID, &revision.QuestionSetID, &revision.Revision, &dataJSON, &actor, &created)
	revision.Data = json.RawMessage(dataJSON)
	if actor.Valid {
		revision.CreatedBy = actor.Int64
	}
	revision.CreatedAt = time.UnixMilli(created)
	return revision, err
}

func (s *Store) GetQuestionSetRevision(ctx context.Context, id string) (game.QuestionSetRevision, error) {
	return s.getRevision(ctx, id)
}

func (s *Store) CreateQuestionSet(ctx context.Context, set game.QuestionSet, revision game.QuestionSetRevision, actor game.Actor) (game.QuestionSet, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return game.QuestionSet{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO question_sets(id,name,category,current_revision_id,version,created_at,updated_at)VALUES(?,?,?,?,1,?,?)`, set.ID, set.Name, set.Category, revision.ID, now, now)
	if err != nil {
		return game.QuestionSet{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO question_set_revisions(id,question_set_id,revision,data_json,created_by,created_at)VALUES(?,?,?,?,?,?)`, revision.ID, set.ID, 1, string(revision.Data), nullInt(actor.UserID), now)
	if err != nil {
		return game.QuestionSet{}, err
	}
	if err = insertAudit(ctx, tx, actor, "question_set.create", "question_set", set.ID, "", nil, set, ""); err != nil {
		return game.QuestionSet{}, err
	}
	if err = tx.Commit(); err != nil {
		return game.QuestionSet{}, err
	}
	created, _, err := s.GetQuestionSet(ctx, set.ID)
	return created, err
}

func (s *Store) SaveQuestionSet(ctx context.Context, set game.QuestionSet, revision game.QuestionSetRevision, expected int64, actor game.Actor) (game.QuestionSet, error) {
	before, _, err := s.GetQuestionSet(ctx, set.ID)
	if err != nil {
		return game.QuestionSet{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return game.QuestionSet{}, err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(ctx, `UPDATE question_sets SET name=?,category=?,current_revision_id=?,version=version+1,updated_at=? WHERE id=? AND version=? AND archived_at IS NULL`, set.Name, set.Category, revision.ID, now, set.ID, expected)
	if err != nil {
		return game.QuestionSet{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return game.QuestionSet{}, gameConflict(expected)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO question_set_revisions(id,question_set_id,revision,data_json,created_by,created_at)VALUES(?,?,?,?,?,?)`, revision.ID, set.ID, before.Revision+1, string(revision.Data), nullInt(actor.UserID), now)
	if err != nil {
		return game.QuestionSet{}, err
	}
	if err = insertAudit(ctx, tx, actor, "question_set.revise", "question_set", set.ID, "", before, set, ""); err != nil {
		return game.QuestionSet{}, err
	}
	if err = tx.Commit(); err != nil {
		return game.QuestionSet{}, err
	}
	updated, _, err := s.GetQuestionSet(ctx, set.ID)
	return updated, err
}

func (s *Store) ArchiveQuestionSet(ctx context.Context, id string, expected int64, actor game.Actor) error {
	before, _, err := s.GetQuestionSet(ctx, id)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	result, err := tx.ExecContext(ctx, `UPDATE question_sets SET archived_at=?,version=version+1,updated_at=? WHERE id=? AND version=?`, now, now, id, expected)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return gameConflict(expected)
	}
	if err := insertAudit(ctx, tx, actor, "question_set.archive", "question_set", id, "", before, map[string]any{"archived": true}, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PinSelectedMatchRevision(ctx context.Context, setID, revisionID, reason string, actor game.Actor) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var matchID, status string
	err = tx.QueryRowContext(ctx, `SELECT m.id,m.status FROM app_state a JOIN matches m ON m.id=a.selected_match_id WHERE a.singleton=1 AND m.question_set_id=?`, setID).Scan(&matchID, &status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status == string(game.StatusCompleted) || status == string(game.StatusArchived) {
		return false, nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE matches SET question_set_revision_id=?,version=version+1,updated_at=? WHERE id=?`, revisionID, time.Now().UnixMilli(), matchID)
	if err != nil {
		return false, err
	}
	if err = insertAudit(ctx, tx, actor, "match.question_revision", "match", matchID, matchID, nil, map[string]any{"question_set_id": setID, "revision_id": revisionID}, reason); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) ListAudit(ctx context.Context, search string, limit int) ([]game.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	pattern := "%" + search + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(actor_user_id,0),action,entity_type,entity_id,COALESCE(match_id,''),COALESCE(before_json,''),COALESCE(after_json,''),reason,client_id,created_at FROM audit_logs WHERE action LIKE ? OR entity_type LIKE ? OR entity_id LIKE ? ORDER BY created_at DESC LIMIT ?`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]game.AuditEntry, 0)
	for rows.Next() {
		var entry game.AuditEntry
		var created int64
		var beforeJSON, afterJSON string
		if err := rows.Scan(&entry.ID, &entry.ActorID, &entry.Action, &entry.EntityType, &entry.EntityID, &entry.MatchID, &beforeJSON, &afterJSON, &entry.Reason, &entry.ClientID, &created); err != nil {
			return nil, err
		}
		entry.Before, entry.After = json.RawMessage(beforeJSON), json.RawMessage(afterJSON)
		entry.CreatedAt = time.UnixMilli(created)
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *Store) AppendAudit(ctx context.Context, entry game.AuditEntry, actor game.Actor) error {
	return s.insertAuditDirect(ctx, actor, entry.Action, entry.EntityType, entry.EntityID, entry.MatchID, entry.Before, entry.After, entry.Reason)
}

func (s *Store) SaveAsset(ctx context.Context, asset game.Asset, actor game.Actor) (game.Asset, error) {
	var existingID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM assets WHERE sha256=?`, asset.SHA256).Scan(&existingID)
	if err == nil {
		return s.GetAsset(ctx, existingID)
	}
	if err != sql.ErrNoRows {
		return game.Asset{}, err
	}
	now := time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `INSERT INTO assets(id,sha256,original_name,media_type,size,created_by,created_at)VALUES(?,?,?,?,?,?,?)`, asset.ID, asset.SHA256, asset.OriginalName, asset.MediaType, asset.Size, nullInt(actor.UserID), now)
	if err != nil {
		return game.Asset{}, err
	}
	_ = s.insertAuditDirect(ctx, actor, "asset.upload", "asset", asset.ID, "", nil, map[string]any{"name": asset.OriginalName, "media_type": asset.MediaType, "size": asset.Size}, "")
	return s.GetAsset(ctx, asset.ID)
}
func (s *Store) DeleteAsset(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM assets WHERE id=?`, id)
	return err
}
func (s *Store) GetAsset(ctx context.Context, id string) (game.Asset, error) {
	var asset game.Asset
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT id,sha256,original_name,media_type,size,created_at FROM assets WHERE id=? AND archived_at IS NULL`, id).Scan(&asset.ID, &asset.SHA256, &asset.OriginalName, &asset.MediaType, &asset.Size, &created)
	asset.CreatedAt = time.UnixMilli(created)
	return asset, err
}
