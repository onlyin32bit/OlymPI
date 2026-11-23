package game

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/settings"
)

type KhoiDongJudgmentRequest struct {
	Command          Command `json:"command"`
	QuestionID       string  `json:"question_id"`
	Verdict          string  `json:"verdict"`
	ExpectedBellSlot int     `json:"expected_bell_slot"`
}

// Shown to the operator when the verdict cue could not start; the judgment itself stands.
const (
	verdictSoundUnavailable = "Đã chấm và chuyển câu, nhưng âm thanh chưa sẵn sàng."
	verdictSoundFailed      = "Đã chấm và chuyển câu, nhưng không phát được âm thanh. Hãy kiểm tra cài đặt âm thanh."
)

type KhoiDongJudgmentResult struct {
	Runtime      RuntimeState `json:"runtime"`
	QuestionID   string       `json:"question_id"`
	Verdict      string       `json:"verdict"`
	Slot         int          `json:"slot"`
	Delta        int          `json:"delta"`
	SoundWarning string       `json:"sound_warning,omitempty"`
}

// JudgeKhoiDong commits the scoring and the question transition together. The
// caller supplies a fresh server-side settings snapshot, never client scoring.
func (s *Service) JudgeKhoiDong(ctx context.Context, request KhoiDongJudgmentRequest, scoring settings.KhoiDongScoring, actor Actor) (KhoiDongJudgmentResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd := request.Command
	if err := s.checkPlayableLocked(cmd); err != nil {
		return KhoiDongJudgmentResult{}, err
	}
	if strings.TrimSpace(cmd.MatchID) == "" || strings.TrimSpace(cmd.CommandID) == "" || cmd.ExpectedEpoch == 0 || cmd.ExpectedVersion == 0 || cmd.ExpectedInputEpoch == 0 || cmd.ExpectedBellEpoch == 0 || cmd.ExpectedInputEpoch != s.state.InputEpoch || cmd.ExpectedBellEpoch != s.state.BellEpoch {
		return KhoiDongJudgmentResult{}, stale(s.state.Version)
	}
	if request.Verdict != "correct" && request.Verdict != "wrong" {
		return KhoiDongJudgmentResult{}, validation("verdict", "invalid verdict")
	}
	state := s.state
	if state.Controller.Phase != controller.PhaseKhoiDong || !state.Controller.IsShowingQuestion || request.QuestionID == "" || state.Controller.SelectedQuestionID != request.QuestionID || state.CurrentQuestion == nil || state.CurrentQuestion.ID != request.QuestionID {
		return KhoiDongJudgmentResult{}, stale(state.Version)
	}
	claim := 0
	for _, contestant := range state.Contestants {
		if contestant.HasRung {
			if claim != 0 || contestant.Slot < 1 || contestant.Slot > 4 {
				return KhoiDongJudgmentResult{}, validation("bell", "invalid bell claim")
			}
			claim = contestant.Slot
		}
	}
	if request.ExpectedBellSlot < 0 || request.ExpectedBellSlot > 4 || request.ExpectedBellSlot != claim {
		return KhoiDongJudgmentResult{}, stale(state.Version)
	}
	parts := strings.Split(request.QuestionID, ".")
	common := len(parts) == 2 && parts[0] == "kd_chung" && state.Controller.Screen == controller.ScreenQuesChung
	owner := 0
	indexText := ""
	if common {
		indexText = parts[1]
	} else if len(parts) == 3 && parts[0] == "kd_rieng" {
		ownerIndex, err := strconv.Atoi(parts[1])
		if err != nil || ownerIndex < 0 || ownerIndex > 3 || strconv.Itoa(ownerIndex) != parts[1] || string(state.Controller.Screen) != fmt.Sprintf("ques_ts%d", ownerIndex+1) {
			return KhoiDongJudgmentResult{}, validation("question_id", "question does not match screen")
		}
		owner, indexText = ownerIndex+1, parts[2]
	} else {
		return KhoiDongJudgmentResult{}, validation("question_id", "question does not match screen")
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 || strconv.Itoa(index) != indexText {
		return KhoiDongJudgmentResult{}, validation("question_id", "invalid question index")
	}
	if common && request.Verdict == "correct" && claim == 0 {
		return KhoiDongJudgmentResult{}, validation("bell", "a bell claim is required")
	}

	// Check durable history, not just the current runtime: reopening a judged
	// question or restarting the process must not make it judgeable again.
	events, err := s.repository.ListEvents(ctx, state.MatchID)
	if err != nil {
		return KhoiDongJudgmentResult{}, err
	}
	label := fmt.Sprintf("Khởi động chung · Câu %d", index+1)
	if !common {
		label = fmt.Sprintf("Khởi động · TS%d câu %d", owner, index+1)
	}
	savedScores := 0
	for _, event := range events {
		if event.Type == "Match.ScoresAdjusted" {
			var payload struct {
				Reason  string                  `json:"reason"`
				Sources []ScoreAdjustmentSource `json:"sources"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return KhoiDongJudgmentResult{}, err
			}
			matchedSource, correction := false, true
			for _, source := range payload.Sources {
				if source.QuestionID == request.QuestionID {
					matchedSource = true
					if source.Kind != "correction" {
						correction = false
					}
				}
			}
			if matchedSource && correction {
				continue
			}
			reason := payload.Reason
			undo := strings.HasPrefix(reason, "Hoàn tác · ")
			if undo {
				reason = strings.TrimPrefix(reason, "Hoàn tác · ")
			}
			matched := matchedSource
			for _, item := range strings.Split(reason, ",") {
				if strings.TrimSpace(item) == label {
					matched = true
					break
				}
			}
			if matched {
				if undo {
					savedScores = max(0, savedScores-1)
				} else {
					savedScores++
				}
			}
		}
		if event.Type != "KhoiDong.Judged" {
			continue
		}
		var prior struct {
			QuestionID string `json:"question_id"`
		}
		if err := json.Unmarshal(event.Payload, &prior); err != nil {
			return KhoiDongJudgmentResult{}, err
		}
		if prior.QuestionID == request.QuestionID {
			return KhoiDongJudgmentResult{}, validation("question_id", "Câu này đã được chấm nhanh; dùng +/- nếu cần điều chỉnh điểm.")
		}
	}
	if savedScores > 0 {
		return KhoiDongJudgmentResult{}, validation("question_id", "Điểm câu này đã ghi; dùng câu tiếp theo hoặc hoàn tác trước khi chấm nhanh.")
	}
	revision, err := s.repository.GetQuestionSetRevision(ctx, state.QuestionRevisionID)
	if err != nil {
		return KhoiDongJudgmentResult{}, err
	}
	var data struct {
		Common     []json.RawMessage   `json:"kd_chung"`
		Individual [][]json.RawMessage `json:"kd_rieng"`
	}
	if err := json.Unmarshal(revision.Data, &data); err != nil {
		return KhoiDongJudgmentResult{}, validation("question", "invalid pinned revision")
	}
	questions := data.Common
	if !common {
		if owner-1 >= len(data.Individual) {
			return KhoiDongJudgmentResult{}, validation("question_id", "question is absent from pinned revision")
		}
		questions = data.Individual[owner-1]
	}
	if index >= len(questions) {
		return KhoiDongJudgmentResult{}, validation("question_id", "question is absent from pinned revision")
	}
	var current QuestionSnapshot
	if json.Unmarshal(questions[index], &current) != nil || strings.TrimSpace(current.Content) == "" {
		return KhoiDongJudgmentResult{}, validation("question_id", "invalid pinned question")
	}
	nextID := ""
	var next *QuestionSnapshot
	if index+1 < len(questions) {
		if common {
			nextID = fmt.Sprintf("kd_chung.%d", index+1)
		} else {
			nextID = fmt.Sprintf("kd_rieng.%d.%d", owner-1, index+1)
		}
		var q QuestionSnapshot
		if json.Unmarshal(questions[index+1], &q) != nil || strings.TrimSpace(q.Content) == "" {
			return KhoiDongJudgmentResult{}, validation("question_id", "invalid next pinned question")
		}
		q.ID = nextID
		if q.Type == "" {
			q.Type = "text"
		}
		next = &q
	}
	slot, delta := owner, 0
	if common {
		slot = claim
		if claim != 0 {
			if request.Verdict == "correct" {
				delta = scoring.CommonCorrect
			} else {
				delta = scoring.CommonWrong
			}
		}
	} else if request.Verdict == "correct" {
		delta = scoring.IndividualCorrect
	} else {
		delta = scoring.IndividualWrong
	}
	previous := cloneRuntime(*state)
	if slot != 0 {
		contestant, err := s.contestantLocked(slot)
		if err != nil {
			return KhoiDongJudgmentResult{}, err
		}
		if common && scoring.CommonFloorZero && delta < 0 {
			delta = max(delta, -max(0, contestant.Score))
		}
		contestant.Score += delta
	}
	s.resetInputsLocked(true)
	state.Timer = TimerState{}
	state.InputEpoch++
	state.BellEpoch++
	state.Controller.SelectedQuestionID = nextID
	state.Controller.IsShowingQuestion = true
	state.Controller.AnswerStatus = []bool{true, true, true, true}
	state.CurrentQuestion = next
	normalizeBell(state, time.Now().UnixMilli())
	payload := map[string]any{"question_id": request.QuestionID, "verdict": request.Verdict, "slot": slot, "delta": delta, "next_question_id": nextID, "end": nextID == ""}
	updated, err := s.repository.SaveRuntime(ctx, *state, "KhoiDong.Judged", payload, actor, cmd.CommandID)
	if err != nil {
		s.state = &previous
		return KhoiDongJudgmentResult{}, err
	}
	// SaveRuntime can acknowledge a previously used command without inserting an
	// event. Never broadcast or play a verdict sound for that acknowledgement.
	if updated.Version != previous.Version+1 {
		s.state = &previous
		return KhoiDongJudgmentResult{}, stale(previous.Version)
	}
	// The verdict cue starts after the commit and before the next question is
	// published, so the sound leads the pause gap: click -> sound -> gap.
	soundWarning := ""
	if s.verdictSound == nil {
		soundWarning = verdictSoundUnavailable
	} else if err := s.verdictSound(request.Verdict, actor); err != nil {
		soundWarning = verdictSoundFailed
	}
	s.cancelTimerLocked()
	s.state = &updated
	s.broadcastLocked()
	return KhoiDongJudgmentResult{Runtime: cloneRuntime(updated), QuestionID: request.QuestionID, Verdict: request.Verdict, Slot: slot, Delta: delta, SoundWarning: soundWarning}, nil
}
