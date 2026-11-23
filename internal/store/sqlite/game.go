package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
)

func (s *Store) ListMatches(ctx context.Context, search string) ([]game.Match, error) {
	pattern := "%" + search + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT m.id,m.name,m.season,m.stage,m.status,
		COALESCE(m.question_set_id,''),COALESCE(m.question_set_revision_id,''),m.version,m.result_json,
		m.created_at,m.updated_at,m.started_at,m.completed_at,m.archived_at,
		CASE WHEN a.selected_match_id=m.id THEN 1 ELSE 0 END
		FROM matches m CROSS JOIN app_state a WHERE m.name LIKE ? OR m.stage LIKE ?
		ORDER BY CASE WHEN a.selected_match_id=m.id THEN 0 ELSE 1 END, m.updated_at DESC`, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]game.Match, 0)
	for rows.Next() {
		match, err := scanMatch(rows)
		if err != nil {
			return nil, err
		}
		match.Contestants, err = s.matchContestants(ctx, match.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, match)
	}
	return result, rows.Err()
}

func (s *Store) GetMatch(ctx context.Context, id string) (game.Match, error) {
	row := s.db.QueryRowContext(ctx, `SELECT m.id,m.name,m.season,m.stage,m.status,
		COALESCE(m.question_set_id,''),COALESCE(m.question_set_revision_id,''),m.version,m.result_json,
		m.created_at,m.updated_at,m.started_at,m.completed_at,m.archived_at,
		CASE WHEN a.selected_match_id=m.id THEN 1 ELSE 0 END
		FROM matches m CROSS JOIN app_state a WHERE m.id=?`, id)
	match, err := scanMatch(row)
	if err != nil {
		return game.Match{}, err
	}
	match.Contestants, err = s.matchContestants(ctx, id)
	return match, err
}

type scanner interface{ Scan(...any) error }

func scanMatch(row scanner) (game.Match, error) {
	var match game.Match
	var status string
	var resultJSON string
	var created, updated int64
	var started, completed, archived sql.NullInt64
	var selected bool
	if err := row.Scan(&match.ID, &match.Name, &match.Season, &match.Stage, &status,
		&match.QuestionSetID, &match.QuestionSetRevisionID, &match.Version, &resultJSON,
		&created, &updated, &started, &completed, &archived, &selected); err != nil {
		return game.Match{}, err
	}
	match.Status = game.MatchStatus(status)
	match.Result = json.RawMessage(resultJSON)
	match.CreatedAt, match.UpdatedAt, match.Selected = time.UnixMilli(created), time.UnixMilli(updated), selected
	match.StartedAt, match.CompletedAt, match.ArchivedAt = nullableTime(started), nullableTime(completed), nullableTime(archived)
	return match, nil
}

func nullableTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.UnixMilli(value.Int64)
	return &result
}

func (s *Store) matchContestants(ctx context.Context, matchID string) ([]game.Contestant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT slot,user_id,name,class FROM match_contestants WHERE match_id=? ORDER BY slot`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contestants := make([]game.Contestant, 0)
	for rows.Next() {
		var contestant game.Contestant
		if err := rows.Scan(&contestant.Slot, &contestant.UserID, &contestant.Name, &contestant.Class); err != nil {
			return nil, err
		}
		contestants = append(contestants, contestant)
	}
	return contestants, rows.Err()
}

func (s *Store) CreateMatch(ctx context.Context, match game.Match, actor game.Actor) (game.Match, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return game.Match{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO matches(id,name,season,stage,status,question_set_id,question_set_revision_id,version,result_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,1,'{}',?,?)`, match.ID, match.Name, match.Season, match.Stage, match.Status,
		nullString(match.QuestionSetID), nullString(match.QuestionSetRevisionID), now, now)
	if err != nil {
		return game.Match{}, err
	}
	for _, contestant := range match.Contestants {
		if _, err := tx.ExecContext(ctx, `INSERT INTO match_contestants(match_id,slot,user_id,name,class) VALUES(?,?,?,?,?)`,
			match.ID, contestant.Slot, contestant.UserID, contestant.Name, contestant.Class); err != nil {
			return game.Match{}, err
		}
	}
	if err := insertAudit(ctx, tx, actor, "match.create", "match", match.ID, match.ID, nil, match, ""); err != nil {
		return game.Match{}, err
	}
	if err := tx.Commit(); err != nil {
		return game.Match{}, err
	}
	return s.GetMatch(ctx, match.ID)
}

func (s *Store) UpdateMatch(ctx context.Context, match game.Match, expected int64, actor game.Actor) (game.Match, error) {
	before, err := s.GetMatch(ctx, match.ID)
	if err != nil {
		return game.Match{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return game.Match{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE matches SET name=?,season=?,stage=?,status=?,question_set_id=?,question_set_revision_id=?,version=version+1,updated_at=? WHERE id=? AND version=? AND status NOT IN ('completed','archived')`,
		match.Name, match.Season, match.Stage, match.Status, nullString(match.QuestionSetID), nullString(match.QuestionSetRevisionID), time.Now().UnixMilli(), match.ID, expected)
	if err != nil {
		return game.Match{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return game.Match{}, gameConflict(expected)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM match_contestants WHERE match_id=?`, match.ID); err != nil {
		return game.Match{}, err
	}
	for _, contestant := range match.Contestants {
		if _, err := tx.ExecContext(ctx, `INSERT INTO match_contestants(match_id,slot,user_id,name,class) VALUES(?,?,?,?,?)`, match.ID, contestant.Slot, contestant.UserID, contestant.Name, contestant.Class); err != nil {
			return game.Match{}, err
		}
	}
	if err := insertAudit(ctx, tx, actor, "match.update", "match", match.ID, match.ID, before, match, ""); err != nil {
		return game.Match{}, err
	}
	if err := tx.Commit(); err != nil {
		return game.Match{}, err
	}
	return s.GetMatch(ctx, match.ID)
}

func (s *Store) SetMatchStatus(ctx context.Context, id string, status game.MatchStatus, expected int64, actor game.Actor) (game.Match, error) {
	before, err := s.GetMatch(ctx, id)
	if err != nil {
		return game.Match{}, err
	}
	now := time.Now().UnixMilli()
	started := "started_at"
	completed := "completed_at"
	query := `UPDATE matches SET status=?,version=version+1,updated_at=?`
	args := []any{status, now}
	if status == game.StatusPlaying && before.StartedAt == nil {
		query += `,` + started + `=?`
		args = append(args, now)
	}
	if status == game.StatusCompleted {
		query += `,` + completed + `=?`
		args = append(args, now)
	}
	if status == game.StatusArchived {
		query += `,archived_at=?`
		args = append(args, now)
	}
	query += ` WHERE id=? AND version=?`
	args = append(args, id, expected)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return game.Match{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return game.Match{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return game.Match{}, gameConflict(expected)
	}
	after := before
	after.Status = status
	after.Version++
	after.UpdatedAt = time.UnixMilli(now)
	if status == game.StatusPlaying && after.StartedAt == nil {
		startedAt := time.UnixMilli(now)
		after.StartedAt = &startedAt
	}
	if status == game.StatusCompleted {
		completedAt := time.UnixMilli(now)
		after.CompletedAt = &completedAt
	}
	if status == game.StatusArchived {
		archivedAt := time.UnixMilli(now)
		after.ArchivedAt = &archivedAt
	}
	if err = insertAudit(ctx, tx, actor, "match.status", "match", id, id, before, after, string(status)); err != nil {
		return game.Match{}, err
	}
	if err = tx.Commit(); err != nil {
		return game.Match{}, err
	}
	return s.GetMatch(ctx, id)
}

func (s *Store) ArchiveMatch(ctx context.Context, id string, expected int64, actor game.Actor) error {
	_, err := s.SetMatchStatus(ctx, id, game.StatusArchived, expected, actor)
	return err
}

func (s *Store) SelectedMatch(ctx context.Context) (string, int64, error) {
	var id sql.NullString
	var epoch int64
	err := s.db.QueryRowContext(ctx, `SELECT selected_match_id,epoch FROM app_state WHERE singleton=1`).Scan(&id, &epoch)
	return id.String, epoch, err
}

func (s *Store) SelectMatch(ctx context.Context, id string, actor game.Actor) (int64, error) {
	before, epoch, err := s.SelectedMatch(ctx)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE app_state SET selected_match_id=?,epoch=epoch+1 WHERE singleton=1`, nullString(id))
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return 0, errors.New("select match failed")
	}
	if err = tx.QueryRowContext(ctx, `SELECT epoch FROM app_state WHERE singleton=1`).Scan(&epoch); err != nil {
		return 0, err
	}
	action, entityID, matchID := "match.select", id, id
	if id == "" {
		action, entityID, matchID = "match.unload", before, before
	}
	if err = insertAudit(ctx, tx, actor, action, "match", entityID, matchID, map[string]any{"selected_match_id": before}, map[string]any{"selected_match_id": id}, ""); err != nil {
		return 0, err
	}
	return epoch, tx.Commit()
}

func (s *Store) SaveRuntime(ctx context.Context, state game.RuntimeState, eventType string, payload any, actor game.Actor, commandID string) (game.RuntimeState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return state, err
	}
	defer tx.Rollback()
	if commandID != "" {
		var existing int64
		err = tx.QueryRowContext(ctx, `SELECT sequence FROM match_events WHERE match_id=? AND command_id=?`, state.MatchID, commandID).Scan(&existing)
		if err == nil {
			// Score mutations and verdicts carry one-shot semantics, including
			// undo identity and sound; never acknowledge a reused ID as a new save.
			if eventType == "KhoiDong.Judged" || eventType == "Match.ScoresAdjusted" {
				return state, fmt.Errorf("duplicate scoring command")
			}
			var raw string
			// A duplicate command acknowledges the latest durable runtime, never
			// its historical snapshot: installing the old state rewinds scores
			// and other contestants' independent inputs after a manual unlock.
			if snapshotErr := tx.QueryRowContext(ctx, `SELECT state_json FROM match_snapshots WHERE match_id=? ORDER BY sequence DESC LIMIT 1`, state.MatchID).Scan(&raw); snapshotErr != nil {
				return state, snapshotErr
			}
			var saved game.RuntimeState
			if jsonErr := json.Unmarshal([]byte(raw), &saved); jsonErr != nil {
				return state, jsonErr
			}
			saved.Epoch = state.Epoch
			return saved, nil
		}
		if err != sql.ErrNoRows {
			return state, err
		}
	}
	var seq int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM match_events WHERE match_id=?`, state.MatchID).Scan(&seq); err != nil {
		return state, err
	}
	expectedVersion := state.Version
	state.Version++
	state.UpdatedAt = time.Now()
	payloadJSON, _ := json.Marshal(payload)
	stateJSON, _ := json.Marshal(state)
	_, err = tx.ExecContext(ctx, `INSERT INTO match_events(match_id,sequence,event_type,payload_json,actor_user_id,client_id,command_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, state.MatchID, seq, eventType, string(payloadJSON), nullInt(actor.UserID), actor.ClientID, nullString(commandID), state.UpdatedAt.UnixMilli())
	if err != nil {
		return state, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO match_snapshots(match_id,sequence,state_json,created_at) VALUES(?,?,?,?)`, state.MatchID, seq, string(stateJSON), state.UpdatedAt.UnixMilli())
	if err != nil {
		return state, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE matches SET version=?,status=?,question_set_id=?,question_set_revision_id=?,updated_at=? WHERE id=? AND version=?`, state.Version, state.Status, nullString(state.QuestionSetID), nullString(state.QuestionRevisionID), state.UpdatedAt.UnixMilli(), state.MatchID, expectedVersion)
	if err != nil {
		return state, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return state, gameConflict(expectedVersion)
	}
	if err = tx.Commit(); err != nil {
		return state, err
	}
	return state, nil
}

func (s *Store) CompleteMatch(ctx context.Context, state game.RuntimeState, eventType string, payload any, result []byte, actor game.Actor, commandID string) (game.RuntimeState, error) {
	before, err := s.GetMatch(ctx, state.MatchID)
	if err != nil {
		return state, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return state, err
	}
	defer tx.Rollback()
	if commandID != "" {
		var existing int64
		err = tx.QueryRowContext(ctx, `SELECT sequence FROM match_events WHERE match_id=? AND command_id=?`, state.MatchID, commandID).Scan(&existing)
		if err == nil {
			var raw string
			if err := tx.QueryRowContext(ctx, `SELECT state_json FROM match_snapshots WHERE match_id=? AND sequence=?`, state.MatchID, existing).Scan(&raw); err != nil {
				return state, err
			}
			var saved game.RuntimeState
			if err := json.Unmarshal([]byte(raw), &saved); err != nil {
				return state, err
			}
			saved.Epoch = state.Epoch
			return saved, nil
		}
		if err != sql.ErrNoRows {
			return state, err
		}
	}

	now := time.Now()
	statusAfter := before
	statusAfter.Status = game.StatusCompleted
	statusAfter.Version = state.Version + 1
	statusAfter.UpdatedAt = now
	completedAt := now
	statusAfter.CompletedAt = &completedAt
	if err := insertAudit(ctx, tx, actor, "match.status", "match", state.MatchID, state.MatchID, before, statusAfter, string(game.StatusCompleted)); err != nil {
		return state, err
	}

	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM match_events WHERE match_id=?`, state.MatchID).Scan(&sequence); err != nil {
		return state, err
	}
	updated := state
	updated.Version = state.Version + 2
	updated.Status = game.StatusCompleted
	updated.UpdatedAt = now
	payloadJSON, _ := json.Marshal(payload)
	stateJSON, _ := json.Marshal(updated)
	if _, err := tx.ExecContext(ctx, `INSERT INTO match_events(match_id,sequence,event_type,payload_json,actor_user_id,client_id,command_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, updated.MatchID, sequence, eventType, string(payloadJSON), nullInt(actor.UserID), actor.ClientID, nullString(commandID), now.UnixMilli()); err != nil {
		return state, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO match_snapshots(match_id,sequence,state_json,created_at) VALUES(?,?,?,?)`, updated.MatchID, sequence, string(stateJSON), now.UnixMilli()); err != nil {
		return state, err
	}
	statusResult, err := tx.ExecContext(ctx, `UPDATE matches SET version=?,status=?,question_set_id=?,question_set_revision_id=?,result_json=?,completed_at=?,updated_at=? WHERE id=? AND version=? AND status=?`,
		updated.Version, updated.Status, nullString(updated.QuestionSetID), nullString(updated.QuestionRevisionID), string(result), now.UnixMilli(), now.UnixMilli(), updated.MatchID, state.Version, game.StatusPaused)
	if err != nil {
		return state, err
	}
	affected, _ := statusResult.RowsAffected()
	if affected != 1 {
		return state, gameConflict(state.Version)
	}
	if err := insertAudit(ctx, tx, actor, "match.result_finalized", "match", updated.MatchID, updated.MatchID, before.Result, json.RawMessage(result), ""); err != nil {
		return state, err
	}
	if err := tx.Commit(); err != nil {
		return state, err
	}
	return updated, nil
}

func (s *Store) LoadRuntime(ctx context.Context, matchID string, epoch int64) (game.RuntimeState, error) {
	match, matchErr := s.GetMatch(ctx, matchID)
	if matchErr != nil {
		return game.RuntimeState{}, matchErr
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT state_json FROM match_snapshots WHERE match_id=? ORDER BY sequence DESC LIMIT 1`, matchID).Scan(&raw)
	if err == nil {
		var state game.RuntimeState
		if json.Unmarshal([]byte(raw), &state) == nil {
			state.Epoch = epoch
			state.MatchName, state.Season, state.Stage = match.Name, match.Season, match.Stage
			state.Version, state.Status = match.Version, match.Status
			state.QuestionSetID, state.QuestionRevisionID = match.QuestionSetID, match.QuestionSetRevisionID
			bySlot := make(map[int]game.ContestantState, len(state.Contestants))
			for _, contestant := range state.Contestants {
				bySlot[contestant.Slot] = contestant
			}
			state.Contestants = make([]game.ContestantState, len(match.Contestants))
			for i, contestant := range match.Contestants {
				runtimeContestant := bySlot[contestant.Slot]
				runtimeContestant.Slot = contestant.Slot
				runtimeContestant.Name = contestant.Name
				runtimeContestant.Class = contestant.Class
				state.Contestants[i] = runtimeContestant
			}
			return state, nil
		}
	}
	if err != nil && err != sql.ErrNoRows {
		return game.RuntimeState{}, err
	}
	contestants := make([]game.ContestantState, len(match.Contestants))
	for i, c := range match.Contestants {
		contestants[i] = game.ContestantState{Slot: c.Slot, Name: c.Name, Class: c.Class}
	}
	return game.RuntimeState{MatchID: match.ID, MatchName: match.Name, Season: match.Season, Stage: match.Stage, Epoch: epoch, Version: match.Version, Status: match.Status, QuestionSetID: match.QuestionSetID, QuestionRevisionID: match.QuestionSetRevisionID, Controller: controller.DefaultState(), Timer: game.TimerState{}, Contestants: contestants, UpdatedAt: time.Now()}, nil
}

func (s *Store) ListEvents(ctx context.Context, matchID string) ([]game.Event, error) {
	return s.ListEventsAfter(ctx, matchID, 0)
}

func (s *Store) ListEventsAfter(ctx context.Context, matchID string, afterSequence int64) ([]game.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.match_id,e.sequence,e.event_type,e.payload_json,COALESCE(e.actor_user_id,0),e.client_id,e.created_at,COALESCE(s.state_json,'') FROM match_events e LEFT JOIN match_snapshots s ON s.match_id=e.match_id AND s.sequence=e.sequence WHERE e.match_id=? AND e.sequence>? ORDER BY e.sequence`, matchID, afterSequence)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]game.Event, 0)
	for rows.Next() {
		var e game.Event
		var created int64
		var payloadJSON string
		var stateJSON string
		if err := rows.Scan(&e.MatchID, &e.Sequence, &e.Type, &payloadJSON, &e.ActorID, &e.ClientID, &created, &stateJSON); err != nil {
			return nil, err
		}
		e.CreatedAt = time.UnixMilli(created)
		e.Payload = json.RawMessage(payloadJSON)
		if stateJSON != "" {
			var state game.RuntimeState
			if json.Unmarshal([]byte(stateJSON), &state) == nil {
				e.State = &state
			}
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) AddAmendment(ctx context.Context, matchID string, result []byte, reason string, actor game.Actor) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var before string
	if err = tx.QueryRowContext(ctx, `SELECT result_json FROM matches WHERE id=? AND status='completed'`, matchID).Scan(&before); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `INSERT INTO result_amendments(id,match_id,result_json,reason,actor_user_id,created_at) VALUES(lower(hex(randomblob(16))),?,?,?,?,?)`, matchID, string(result), reason, nullInt(actor.UserID), now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE matches SET result_json=?,version=version+1,updated_at=? WHERE id=?`, string(result), now, matchID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, actor, "match.result_amended", "match", matchID, matchID, json.RawMessage(before), json.RawMessage(result), reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FinalizeResult(ctx context.Context, matchID string, result []byte, actor game.Actor) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var before string
	if err = tx.QueryRowContext(ctx, `SELECT result_json FROM matches WHERE id=? AND status='completed'`, matchID).Scan(&before); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE matches SET result_json=?,updated_at=? WHERE id=?`, string(result), time.Now().UnixMilli(), matchID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, actor, "match.result_finalized", "match", matchID, matchID, json.RawMessage(before), json.RawMessage(result), ""); err != nil {
		return err
	}
	return tx.Commit()
}

func gameConflict(version int64) error { return fmt.Errorf("version conflict (expected %d)", version) }
func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullInt(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func insertAudit(ctx context.Context, tx *sql.Tx, actor game.Actor, action, entityType, entityID, matchID string, before, after any, reason string) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,entity_type,entity_id,match_id,before_json,after_json,reason,client_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, nullInt(actor.UserID), action, entityType, entityID, nullString(matchID), string(beforeJSON), string(afterJSON), reason, actor.ClientID, time.Now().UnixMilli())
	return err
}
func (s *Store) insertAuditDirect(ctx context.Context, actor game.Actor, action, entityType, entityID, matchID string, before, after any, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insertAudit(ctx, tx, actor, action, entityType, entityID, matchID, before, after, reason); err != nil {
		return err
	}
	return tx.Commit()
}
