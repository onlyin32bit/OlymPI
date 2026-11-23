package game

import (
	"context"
	"encoding/json"
	"strings"
)

type scoreSavePayload struct {
	ScoreSaveID string                  `json:"score_save_id"`
	UndoOf      string                  `json:"undo_of"`
	Adjustments []ScoreAdjustment       `json:"adjustments"`
	Reason      string                  `json:"reason"`
	Sources     []ScoreAdjustmentSource `json:"sources,omitempty"`
}

// UndoScores resolves the saved adjustments from durable history while holding
// the same mutex used by saves and quick judgments.
func (s *Service) UndoScores(ctx context.Context, undoOf string, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusCompleted || s.state.Status == StatusArchived {
		return RuntimeState{}, validation("match", "completed matches require a result amendment")
	}
	if undoOf == "" || strings.TrimSpace(undoOf) != undoOf {
		return RuntimeState{}, validation("undo_of", "Mã bản lưu không hợp lệ; hãy điều chỉnh điểm thủ công.")
	}
	events, err := s.repository.ListEvents(ctx, s.state.MatchID)
	if err != nil {
		return RuntimeState{}, err
	}
	var original *scoreSavePayload
	undone := false
	for _, event := range events {
		if event.Type != "Match.ScoresAdjusted" {
			continue
		}
		var payload scoreSavePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return RuntimeState{}, err
		}
		if payload.ScoreSaveID == undoOf {
			if original != nil || payload.UndoOf != "" {
				return RuntimeState{}, validation("undo_of", "Mã bản lưu bị trùng; hãy điều chỉnh điểm thủ công.")
			}
			copy := payload
			original = &copy
		} else if original != nil && payload.UndoOf == undoOf {
			undone = true
		}
	}
	if original == nil {
		return RuntimeState{}, validation("undo_of", "Không tìm thấy bản lưu; hãy điều chỉnh điểm thủ công.")
	}
	if undone {
		return RuntimeState{}, validation("undo_of", "Bản lưu đã được hoàn tác; hãy điều chỉnh điểm thủ công nếu cần.")
	}
	if len(original.Adjustments) == 0 || len(original.Adjustments) > len(s.state.Contestants) {
		return RuntimeState{}, validation("undo_of", "Bản lưu không hợp lệ; hãy điều chỉnh điểm thủ công.")
	}
	seen := make(map[int]bool, len(original.Adjustments))
	for _, adjustment := range original.Adjustments {
		if adjustment.Delta == 0 || adjustment.Delta < -1000 || adjustment.Delta > 1000 || seen[adjustment.Slot] {
			return RuntimeState{}, validation("undo_of", "Bản lưu không hợp lệ; hãy điều chỉnh điểm thủ công.")
		}
		seen[adjustment.Slot] = true
		if _, err := s.contestantLocked(adjustment.Slot); err != nil {
			return RuntimeState{}, err
		}
	}
	previous := cloneRuntime(*s.state)
	result := make([]map[string]int, 0, len(original.Adjustments))
	for _, adjustment := range original.Adjustments {
		contestant, _ := s.contestantLocked(adjustment.Slot)
		contestant.Score -= adjustment.Delta
		result = append(result, map[string]int{"slot": adjustment.Slot, "delta": -adjustment.Delta, "score": contestant.Score})
	}
	payload := map[string]any{"adjustments": result, "reason": "Hoàn tác · " + original.Reason, "undo_of": undoOf}
	if len(original.Sources) > 0 {
		payload["sources"] = original.Sources
	}
	updated, err := s.persistLocked(ctx, "Match.ScoresAdjusted", payload, actor, command.CommandID)
	if err != nil {
		s.state = &previous
		return RuntimeState{}, err
	}
	if updated.Version != previous.Version+1 {
		s.state = &previous
		return RuntimeState{}, stale(previous.Version)
	}
	s.broadcastLocked()
	return updated, nil
}
