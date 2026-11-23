package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

type Publisher func(room, event string, data any)

// Observer receives the full previous and current runtime after every
// broadcast. It runs while the service lock is held: it must not block and
// must not call back into the Service synchronously.
type Observer func(previous, current RuntimeState)

type Service struct {
	mu              sync.Mutex
	repository      Repository
	publish         Publisher
	fixedUsers      map[int]auth.UserDTO
	state           *RuntimeState
	timerCancel     context.CancelFunc
	timerGeneration uint64
	// Về đích video auto-finish schedule (server clock).
	videoCancel      context.CancelFunc
	videoGeneration  uint64
	videoScheduleKey string
	videoHoldSession uint64
	videoHoldUntil   int64
	observer         Observer
	// verdictSound starts the Khởi động verdict cue inside JudgeKhoiDong's critical section.
	verdictSound func(verdict string, actor Actor) error
	// lastObserved is the runtime last handed to the observer.
	lastObserved *RuntimeState
}

func NewService(ctx context.Context, repository Repository, fixedUsers []auth.UserDTO, publish Publisher) (*Service, error) {
	service := &Service{repository: repository, publish: publish, fixedUsers: make(map[int]auth.UserDTO)}
	for _, user := range fixedUsers {
		service.fixedUsers[user.FixedSlot] = user
	}
	selected, epoch, err := repository.SelectedMatch(ctx)
	if err != nil {
		return nil, fmt.Errorf("load selected match: %w", err)
	}
	if selected != "" {
		state, err := repository.LoadRuntime(ctx, selected, epoch)
		if err != nil {
			return nil, fmt.Errorf("restore selected match: %w", err)
		}
		state.InputEpoch = max(1, state.InputEpoch)
		state.BellEpoch = max(1, state.BellEpoch)
		if len(state.Controller.AnswerStatus) == 0 {
			state.Controller.AnswerStatus = []bool{true, true, true, true}
		}
		if state.Status == StatusPlaying {
			// A crash leaves no shutdown snapshot: freeze at the last durable
			// write so downtime is never counted against the timer or video.
			frozenAt := time.Now().UnixMilli()
			if !state.UpdatedAt.IsZero() && state.UpdatedAt.UnixMilli() < frozenAt {
				frozenAt = state.UpdatedAt.UnixMilli()
			}
			state.Timer = pausedTimer(state.Timer, frozenAt)
			state.Controller.VeDichVideo = frozenVideo(state.Controller.VeDichVideo, frozenAt)
			state.Status = StatusPaused
			state.BellEpoch++
			normalizeBell(&state, time.Now().UnixMilli())
			state, err = repository.SaveRuntime(ctx, state, "Match.RecoveredPaused", map[string]any{"reason": "server_restart"}, Actor{}, "")
			if err != nil {
				return nil, fmt.Errorf("pause recovered match: %w", err)
			}
		}
		normalizeBell(&state, time.Now().UnixMilli())
		service.state = &state
	}
	return service, nil
}

func (s *Service) ListMatches(ctx context.Context, search string) ([]Match, error) {
	return s.repository.ListMatches(ctx, strings.TrimSpace(search))
}
func (s *Service) GetMatch(ctx context.Context, id string) (Match, error) {
	return s.repository.GetMatch(ctx, id)
}
func (s *Service) ListEvents(ctx context.Context, id string) ([]Event, error) {
	return s.repository.ListEvents(ctx, id)
}
func (s *Service) ListEventsAfter(ctx context.Context, id string, afterSequence int64) ([]Event, error) {
	if afterSequence < 0 {
		return nil, validation("after_sequence", "event sequence must be non-negative")
	}
	return s.repository.ListEventsAfter(ctx, id, afterSequence)
}
func (s *Service) ListAudit(ctx context.Context, search string, limit int) ([]AuditEntry, error) {
	return s.repository.ListAudit(ctx, strings.TrimSpace(search), limit)
}
func (s *Service) RecordAudit(ctx context.Context, entry AuditEntry, actor Actor) error {
	return s.repository.AppendAudit(ctx, entry, actor)
}
func (s *Service) SaveAsset(ctx context.Context, asset Asset, actor Actor) (Asset, error) {
	if asset.ID == "" {
		asset.ID = uuid.NewString()
	}
	return s.repository.SaveAsset(ctx, asset, actor)
}
func (s *Service) GetAsset(ctx context.Context, id string) (Asset, error) {
	return s.repository.GetAsset(ctx, id)
}
func (s *Service) DeleteAsset(ctx context.Context, id string) error {
	return s.repository.DeleteAsset(ctx, id)
}

func (s *Service) AmendResult(ctx context.Context, matchID string, result json.RawMessage, reason string, actor Actor) error {
	reason = strings.TrimSpace(reason)
	if !json.Valid(result) || reason == "" {
		return validation("amendment", "a valid result and reason are required")
	}
	match, err := s.repository.GetMatch(ctx, matchID)
	if err != nil {
		return notFound("match")
	}
	if match.Status != StatusCompleted {
		return validation("match", "only completed matches can be amended")
	}
	return s.repository.AddAmendment(ctx, matchID, result, reason, actor)
}

func (s *Service) CreateMatch(ctx context.Context, input CreateMatchInput, actor Actor) (Match, error) {
	match, err := s.validatedMatch(input)
	if err != nil {
		return Match{}, err
	}
	match.ID = uuid.NewString()
	if match.QuestionSetID != "" {
		set, revision, err := s.repository.GetQuestionSet(ctx, match.QuestionSetID)
		if err != nil || set.ArchivedAt != nil {
			return Match{}, validation("question_set_id", "question set not found")
		}
		match.QuestionSetRevisionID = revision.ID
		match.Status = StatusReady
	}
	return s.repository.CreateMatch(ctx, match, actor)
}

func (s *Service) UpdateMatch(ctx context.Context, id string, input UpdateMatchInput, actor Actor) (Match, error) {
	match, err := s.validatedMatch(CreateMatchInput{Name: input.Name, Season: input.Season, Stage: input.Stage, QuestionSetID: input.QuestionSetID, Contestants: input.Contestants})
	if err != nil {
		return Match{}, err
	}
	match.ID = id
	current, err := s.repository.GetMatch(ctx, id)
	if err != nil {
		return Match{}, notFound("match")
	}
	match.Status = current.Status
	match.QuestionSetRevisionID = current.QuestionSetRevisionID
	if input.QuestionSetID != current.QuestionSetID {
		if input.QuestionSetID == "" {
			match.QuestionSetRevisionID = ""
			match.Status = StatusDraft
		} else {
			set, revision, err := s.repository.GetQuestionSet(ctx, input.QuestionSetID)
			if err != nil || set.ArchivedAt != nil {
				return Match{}, validation("question_set_id", "question set not found")
			}
			match.QuestionSetRevisionID = revision.ID
			if match.Status == StatusDraft {
				match.Status = StatusReady
			}
		}
	}
	updated, err := s.repository.UpdateMatch(ctx, match, input.Version, actor)
	if err != nil {
		return Match{}, conflict(err)
	}
	s.mu.Lock()
	if s.state != nil && s.state.MatchID == id {
		s.state.QuestionSetID = updated.QuestionSetID
		s.state.QuestionRevisionID = updated.QuestionSetRevisionID
		s.state.Version = updated.Version
		s.state.Contestants = contestantStates(updated.Contestants, s.state.Contestants)
		if _, persistErr := s.persistLocked(ctx, "Match.ConfigurationChanged", map[string]any{"question_set_id": updated.QuestionSetID, "question_revision_id": updated.QuestionSetRevisionID, "contestants": updated.Contestants}, actor, ""); persistErr != nil {
			s.mu.Unlock()
			return Match{}, persistErr
		}
		s.broadcastLocked()
	}
	s.mu.Unlock()
	return s.repository.GetMatch(ctx, id)
}

func (s *Service) DuplicateMatch(ctx context.Context, id string, actor Actor) (Match, error) {
	source, err := s.repository.GetMatch(ctx, id)
	if err != nil {
		return Match{}, notFound("match")
	}
	input := CreateMatchInput{Name: source.Name + " (bản sao)", Season: source.Season, Stage: source.Stage, QuestionSetID: source.QuestionSetID, Contestants: source.Contestants}
	duplicate, err := s.validatedMatch(input)
	if err != nil {
		return Match{}, err
	}
	duplicate.ID = uuid.NewString()
	duplicate.QuestionSetRevisionID = source.QuestionSetRevisionID
	duplicate.Status = StatusDraft
	if duplicate.QuestionSetRevisionID != "" {
		duplicate.Status = StatusReady
	}
	return s.repository.CreateMatch(ctx, duplicate, actor)
}

func (s *Service) ArchiveMatch(ctx context.Context, id string, version int64, actor Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != nil && s.state.MatchID == id {
		return &opsocket.Error{Code: "CONFLICT", Message: "cannot archive the selected match"}
	}
	if err := s.repository.ArchiveMatch(ctx, id, version, actor); err != nil {
		return conflict(err)
	}
	return nil
}

func (s *Service) SelectMatch(ctx context.Context, id string, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != nil && s.state.Status == StatusPlaying {
		return RuntimeState{}, &opsocket.Error{Code: "MATCH_PLAYING", Message: "pause the current match before switching"}
	}
	if s.state != nil && s.state.MatchID == id {
		normalizeBell(s.state, time.Now().UnixMilli())
		return cloneRuntime(*s.state), nil
	}
	match, err := s.repository.GetMatch(ctx, id)
	if err != nil {
		return RuntimeState{}, notFound("match")
	}
	if match.Status == StatusArchived {
		return RuntimeState{}, validation("match", "archived matches cannot be selected")
	}
	previousID := ""
	if s.state != nil {
		previousID = s.state.MatchID
		if _, err = s.persistLocked(ctx, "Match.Unloaded", map[string]any{"next_match_id": id}, actor, ""); err != nil {
			return RuntimeState{}, err
		}
	}
	epoch, err := s.repository.SelectMatch(ctx, id, actor)
	if err != nil {
		return RuntimeState{}, err
	}
	state, err := s.repository.LoadRuntime(ctx, id, epoch)
	if err != nil {
		return RuntimeState{}, err
	}
	state.Epoch = epoch
	state.InputEpoch = max(1, state.InputEpoch)
	state.BellEpoch = max(1, state.BellEpoch)
	if len(state.Controller.AnswerStatus) == 0 {
		state.Controller.AnswerStatus = []bool{true, true, true, true}
	}
	s.cancelTimerLocked()
	s.state = &state
	loaded, err := s.persistLocked(ctx, "Match.Loaded", map[string]any{"previous_match_id": previousID}, actor, "")
	if err != nil {
		return RuntimeState{}, err
	}
	s.broadcastLocked()
	return loaded, nil
}

func (s *Service) UnloadMatch(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return RuntimeState{}, nil
	}
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusPlaying {
		return RuntimeState{}, &opsocket.Error{Code: "MATCH_PLAYING", Message: "pause the current match before unloading"}
	}
	if _, err := s.persistLocked(ctx, "Match.Unloaded", map[string]any{"next_match_id": ""}, actor, command.CommandID); err != nil {
		return RuntimeState{}, err
	}
	if _, err := s.repository.SelectMatch(ctx, "", actor); err != nil {
		return RuntimeState{}, err
	}
	s.cancelTimerLocked()
	s.state = nil
	s.broadcastEmptyLocked()
	return RuntimeState{}, nil
}

func (s *Service) Transition(ctx context.Context, target MatchStatus, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if !allowedTransition(s.state.Status, target) {
		return RuntimeState{}, &opsocket.Error{Code: "INVALID_STATE", Message: fmt.Sprintf("cannot change match from %s to %s", s.state.Status, target), Version: s.state.Version}
	}
	if target == StatusReady && s.state.QuestionRevisionID == "" {
		return RuntimeState{}, validation("question_set", "a question set is required")
	}
	if target == StatusCompleted {
		s.cancelTimerLocked()
		s.state.Status = target
		s.state.Timer = TimerState{}
		s.state.BellEpoch++
		normalizeBell(s.state, time.Now().UnixMilli())
		result, marshalErr := json.Marshal(struct {
			Contestants []ContestantState `json:"contestants"`
			FinalizedAt time.Time         `json:"finalized_at"`
		}{Contestants: s.state.Contestants, FinalizedAt: time.Now().UTC()})
		if marshalErr != nil {
			return RuntimeState{}, fmt.Errorf("marshal final result: %w", marshalErr)
		}
		updated, err := s.repository.CompleteMatch(ctx, *s.state, "Match.Completed", map[string]any{"status": target}, result, actor, command.CommandID)
		if err != nil {
			if restored, restoreErr := s.repository.LoadRuntime(ctx, s.state.MatchID, s.state.Epoch); restoreErr == nil {
				s.state = &restored
			}
			return RuntimeState{}, err
		}
		s.state = &updated
		s.broadcastLocked()
		return updated, nil
	}
	match, err := s.repository.SetMatchStatus(ctx, s.state.MatchID, target, s.state.Version, actor)
	if err != nil {
		return RuntimeState{}, conflict(err)
	}
	s.state.Status = target
	s.state.BellEpoch++
	s.state.Version = match.Version
	if target == StatusPaused {
		s.pauseTimerLocked()
		s.state.Controller.VeDichVideo = frozenVideo(s.state.Controller.VeDichVideo, time.Now().UnixMilli())
	} else if target == StatusPlaying && s.state.Controller.VeDichVideo.Status == controller.VeDichVideoPlaying &&
		s.state.Controller.VeDichVideo.StartedAt == 0 {
		s.state.Controller.VeDichVideo.StartedAt = time.Now().UnixMilli()
	}
	updated, err := s.persistLocked(ctx, "Match."+titleStatus(target), map[string]any{"status": target}, actor, command.CommandID)
	if err != nil {
		return RuntimeState{}, err
	}
	s.broadcastLocked()
	return updated, nil
}

func allowedTransition(from, to MatchStatus) bool {
	switch from {
	case StatusDraft:
		return to == StatusReady
	case StatusReady:
		return to == StatusPlaying
	case StatusPlaying:
		return to == StatusPaused
	case StatusPaused:
		return to == StatusPlaying || to == StatusCompleted
	default:
		return false
	}
}
func titleStatus(status MatchStatus) string {
	value := string(status)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (s *Service) Current(principal opsocket.Principal) RuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return RuntimeState{}
	}
	return filteredState(*s.state, principal)
}

func (s *Service) UpdateController(ctx context.Context, state controller.State, command Command, actor Actor) (RuntimeState, error) {
	if err := controller.ValidateState(state); err != nil {
		return RuntimeState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPlayableLocked(command); err != nil {
		return RuntimeState{}, err
	}
	return s.updateControllerLocked(ctx, state, command, actor, "Controller.Updated")
}

func (s *Service) ResetController(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPlayableLocked(command); err != nil {
		return RuntimeState{}, err
	}
	return s.updateControllerLocked(ctx, controller.DefaultState(), command, actor, "Controller.Reset")
}

func (s *Service) updateControllerLocked(ctx context.Context, state controller.State, command Command, actor Actor, event string, action ...any) (RuntimeState, error) {
	if err := controller.ValidateState(state); err != nil {
		return RuntimeState{}, err
	}
	normalizeBell(s.state, time.Now().UnixMilli())
	previous := cloneRuntime(*s.state)
	phaseChanged := state.Phase != previous.Controller.Phase
	screenChanged := state.Screen != previous.Controller.Screen
	// The video clock is server-owned (Controller.VeDichVideo); a cached full
	// controller state must not rewind or restart it.
	state.VeDichVideo = previous.Controller.VeDichVideo
	if phaseChanged && controller.IsRound(state.Phase) {
		state.SelectedQuestionID = ""
		state.VCNVState = controller.VCNVState{RowStates: []string{"u", "u", "u", "u"}, ImageOpenState: []bool{false, false, false, false, false}}
		state.VeDichContestantsState = make([]controller.VeDichContestantState, 4)
		for i := range state.VeDichContestantsState {
			state.VeDichContestantsState[i].SelectedQuestionSet = []string{}
		}
	}
	if state.Phase == controller.PhaseVeDich {
		for slot := 1; slot <= 4; slot++ {
			if string(state.Screen) == fmt.Sprintf("ques_ts%d", slot) || string(state.Screen) == fmt.Sprintf("pre_ques_ts%d", slot) {
				state.SelectedContestant = slot
			}
		}
	}
	questionChanged := state.SelectedQuestionID != previous.Controller.SelectedQuestionID
	if phaseChanged || questionChanged {
		state.IsShowingQuestion = false
		state.IsAllowingBell = false
		state.AnswerStatus = []bool{true, true, true, true}
	}
	if screenChanged && !controller.IsQuestionScreen(state.Screen) {
		state.IsShowingQuestion = false
		state.IsAllowingBell = false
	}
	if len(state.AnswerStatus) == 0 {
		state.AnswerStatus = []bool{true, true, true, true}
	}
	if phaseChanged || questionChanged || screenChanged {
		state.VeDichVideo = controller.VeDichVideoState{Status: controller.VeDichVideoIdle, Volume: previous.Controller.VeDichVideo.Volume}
		if state.VeDichVideo.Volume == 0 {
			state.VeDichVideo.Volume = 1
		}
	}
	// Cached clients may send their old manual bell flag. It is derived, never
	// an operator-controlled switch, including in generic no-op detection.
	candidate := previous
	candidate.Controller = state
	candidate.Controller.IsAllowingBell = bellAvailable(candidate, time.Now().UnixMilli())
	state.IsAllowingBell = candidate.Controller.IsAllowingBell
	if event == "Controller.Updated" && controllersEqual(state, previous.Controller) {
		return previous, nil
	}
	if phaseChanged || questionChanged {
		s.resetInputsLocked(phaseChanged || state.Phase != controller.PhaseVCNV)
		s.state.InputEpoch++
	} else if screenChanged && (state.Screen == controller.ScreenTestBell || previous.Controller.Screen == controller.ScreenTestBell) {
		// Bell-check rings never carry into (or out of) the bell test.
		s.clearBellClaimsLocked(false)
	}
	if state.SelectedQuestionID == "" {
		s.state.CurrentQuestion = nil
	} else if questionChanged || s.state.CurrentQuestion == nil {
		question, err := s.resolveQuestion(ctx, state.SelectedQuestionID)
		if err != nil {
			s.state = &previous
			return RuntimeState{}, err
		}
		s.state.CurrentQuestion = &question
	}
	if phaseChanged || questionChanged || screenChanged {
		s.cancelTimerLocked()
		if phaseChanged || questionChanged {
			s.state.Timer = TimerState{}
		} else {
			s.state.Timer = frozenTimer(previous.Timer, time.Now().UnixMilli())
			s.state.Timer.IsPaused = false
		}
	}
	s.state.Controller = state
	if bellBoundaryChanged(previous, *s.state) {
		s.state.BellEpoch++
	}
	normalizeBell(s.state, time.Now().UnixMilli())
	state = s.state.Controller
	payload := struct {
		controller.State
		Action any `json:"action,omitempty"`
	}{State: state}
	if len(action) > 0 {
		payload.Action = action[0]
	}
	updated, err := s.persistLocked(ctx, event, payload, actor, command.CommandID)
	if err != nil {
		s.state = &previous
		if previous.Timer.IsRunning && !previous.Timer.IsPaused {
			s.runTimerLocked(timerRemaining(previous.Timer, time.Now().UnixMilli()))
		}
		return RuntimeState{}, err
	}
	s.broadcastLocked()
	return updated, nil
}

// Answer windows are per question; VCNV's bell records and manual locks are
// per phase. Timer operations never erase a question's winning bell claim.
func (s *Service) resetInputsLocked(resetBells bool) {
	for i := range s.state.Contestants {
		contestant := &s.state.Contestants[i]
		if resetBells || !contestant.HasRung || contestant.Answer != "" {
			contestant.TimeMS = 0
		}
		contestant.Answer = ""
		contestant.Locked = false
		if resetBells {
			contestant.BellLocked = false
			contestant.HasRung = false
		}
	}
}

func (s *Service) clearBellClaimsLocked(unlock bool) {
	for i := range s.state.Contestants {
		contestant := &s.state.Contestants[i]
		// A typed answer's submission time is ranking data, not a bell time.
		if contestant.Answer == "" {
			contestant.TimeMS = 0
		}
		contestant.HasRung = false
		if unlock {
			contestant.BellLocked = false
		}
	}
}

// ClearBells is the operator's bell reset (legacy "Xóa chuông"): every
// contestant can ring again, in any phase.
func (s *Service) ClearBells(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusCompleted || s.state.Status == StatusArchived {
		return RuntimeState{}, validation("match", "completed matches are immutable")
	}
	s.clearBellClaimsLocked(true)
	s.state.BellEpoch++
	updated, err := s.persistLocked(ctx, "Contestant.BellsCleared", nil, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

// ClearAnswers clears every contestant's typed answer and answer time without
// touching bell claims or manual lockouts.
func (s *Service) ClearAnswers(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusCompleted || s.state.Status == StatusArchived {
		return RuntimeState{}, validation("match", "completed matches are immutable")
	}
	previous := cloneRuntime(*s.state)
	for i := range s.state.Contestants {
		contestant := &s.state.Contestants[i]
		if !contestant.HasRung || contestant.Answer != "" {
			contestant.TimeMS = 0
		}
		contestant.Answer = ""
	}
	s.state.Controller.AnswerStatus = []bool{true, true, true, true}
	s.state.InputEpoch++
	updated, err := s.persistLocked(ctx, "Contestant.AnswersCleared", nil, actor, command.CommandID)
	if err != nil {
		s.state = &previous
		return RuntimeState{}, err
	}
	s.broadcastLocked()
	return updated, nil
}

const (
	stealDurationMS    int64 = 5_000
	obstacleDurationMS int64 = 15_000
	// veDichVideoHoldMS is the final-frame hold before the answer timer starts.
	veDichVideoHoldMS int64 = 2_000
)

const (
	TimerKindQuestion = "question"
	TimerKindSteal    = "steal"
	TimerKindObstacle = "obstacle"
)

func hasBellClaim(state RuntimeState) bool {
	for _, contestant := range state.Contestants {
		if contestant.HasRung {
			return true
		}
	}
	return false
}

// The owner is encoded by the question-set slot, never SelectedContestant.
func veDichQuestionOwner(state RuntimeState) int {
	if state.Controller.Phase != controller.PhaseVeDich || state.CurrentQuestion == nil ||
		state.CurrentQuestion.ID != state.Controller.SelectedQuestionID {
		return 0
	}
	parts := strings.Split(state.CurrentQuestion.ID, ".")
	if len(parts) != 4 || parts[0] != "vd" || (parts[2] != "20" && parts[2] != "30") {
		return 0
	}
	index, err := strconv.Atoi(parts[1])
	question, questionErr := strconv.Atoi(parts[3])
	if err != nil || index < 0 || index > 3 || questionErr != nil || question < 0 {
		return 0
	}
	return index + 1
}

func activeVeDichQuestion(state RuntimeState) bool {
	owner := veDichQuestionOwner(state)
	return owner != 0 && state.Controller.IsShowingQuestion &&
		string(state.Controller.Screen) == fmt.Sprintf("ques_ts%d", owner)
}

func bellAvailable(state RuntimeState, now int64) bool {
	if state.Status != StatusPlaying {
		return false
	}
	if state.Controller.Phase == controller.PhaseVCNV {
		// The obstacle bell spans the entire playing phase, not answer windows.
		return true
	}
	if hasBellClaim(state) {
		return false
	}
	switch state.Controller.Phase {
	case controller.PhaseKhoiDong:
		if state.Controller.Screen == controller.ScreenTestBell {
			// Legacy bell check: one ring at a time until the operator clears.
			return true
		}
		if state.Controller.Screen != controller.ScreenQuesChung || !state.Controller.IsShowingQuestion ||
			state.CurrentQuestion == nil || state.CurrentQuestion.ID != state.Controller.SelectedQuestionID {
			return false
		}
		parts := strings.Split(state.CurrentQuestion.ID, ".")
		if len(parts) != 2 || parts[0] != "kd_chung" {
			return false
		}
		index, err := strconv.Atoi(parts[1])
		return err == nil && index >= 0
	case controller.PhaseVeDich:
		video := state.Controller.VeDichVideo.Status
		return activeVeDichQuestion(state) &&
			(video == "" || video == controller.VeDichVideoIdle) &&
			state.Timer.DurationMS == stealDurationMS && state.Timer.IsRunning &&
			!state.Timer.IsPaused && timerRemaining(state.Timer, now) > 0
	default:
		return false
	}
}

func normalizeBell(state *RuntimeState, now int64) {
	state.Controller.IsAllowingBell = bellAvailable(*state, now)
}

func bellBoundaryChanged(previous, current RuntimeState) bool {
	if previous.Status != current.Status || previous.Controller.Phase != current.Controller.Phase {
		return true
	}
	if current.Controller.Phase == controller.PhaseVCNV {
		return false
	}
	if previous.Controller.SelectedQuestionID != current.Controller.SelectedQuestionID ||
		previous.Controller.Screen != current.Controller.Screen ||
		previous.Controller.IsShowingQuestion != current.Controller.IsShowingQuestion {
		return true
	}
	return current.Controller.Phase == controller.PhaseVeDich &&
		(previous.Timer != current.Timer || previous.Controller.VeDichVideo.Status != current.Controller.VeDichVideo.Status)
}

func (s *Service) mutateController(ctx context.Context, command Command, actor Actor, event string, mutate func(*controller.State) error, action ...any) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPlayableLocked(command); err != nil {
		return RuntimeState{}, err
	}
	state := cloneController(s.state.Controller)
	if err := mutate(&state); err != nil {
		return RuntimeState{}, err
	}
	return s.updateControllerLocked(ctx, state, command, actor, event, action...)
}

func cloneController(state controller.State) controller.State {
	raw, _ := json.Marshal(state)
	var result controller.State
	_ = json.Unmarshal(raw, &result)
	return result
}

func controllersEqual(left, right controller.State) bool {
	normalize := func(state controller.State) controller.State {
		if len(state.AnswerStatus) == 0 {
			state.AnswerStatus = []bool{true, true, true, true}
		}
		state.VeDichContestantsState = append([]controller.VeDichContestantState(nil), state.VeDichContestantsState...)
		for i := range state.VeDichContestantsState {
			if len(state.VeDichContestantsState[i].SelectedQuestionSet) == 0 {
				state.VeDichContestantsState[i].SelectedQuestionSet = nil
			}
		}
		return state
	}
	return reflect.DeepEqual(normalize(left), normalize(right))
}

func (s *Service) SetVCNVStarted(ctx context.Context, started bool, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.VCNVStarted", func(state *controller.State) error {
		state.VCNVState.HasStarted = started
		return nil
	})
}

func (s *Service) SetVCNVRowState(ctx context.Context, row int, value string, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.VCNVRowStateChanged", func(state *controller.State) error {
		if row < 1 || row > 4 {
			return validation("row", "VCNV row must be between 1 and 4")
		}
		if value != "u" && value != "s" && value != "c" && value != "w" {
			return validation("state", "VCNV row state must be u, s, c, or w")
		}
		state.VCNVState.RowStates[row-1] = value
		return nil
	}, map[string]any{"row": row, "state": value})
}

func (s *Service) SetVCNVPanelOpen(ctx context.Context, index int, open bool, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.VCNVPanelChanged", func(state *controller.State) error {
		if index < 0 || index >= 5 {
			return validation("index", "VCNV panel index must be between 0 and 4")
		}
		state.VCNVState.ImageOpenState[index] = open
		return nil
	})
}

func (s *Service) SetVCNVSolved(ctx context.Context, solved bool, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.VCNVSolved", func(state *controller.State) error {
		state.VCNVState.HasSolved = solved
		return nil
	}, map[string]any{"solved": solved})
}

func (s *Service) SetAnswerStatus(ctx context.Context, slot int, correct bool, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.AnswerStatusChanged", func(state *controller.State) error {
		if slot < 1 || slot > 4 {
			return validation("slot", "contestant slot must be between 1 and 4")
		}
		if state.Phase != controller.PhaseTangToc && state.Phase != controller.PhaseVCNV {
			return validation("phase", "answer review is only available in VCNV and Tăng tốc")
		}
		if len(state.AnswerStatus) != 4 {
			state.AnswerStatus = []bool{true, true, true, true}
		}
		state.AnswerStatus[slot-1] = correct
		return nil
	}, map[string]any{"slot": slot, "correct": correct})
}

func (s *Service) SelectVeDichQuestionSet(ctx context.Context, slot int, questionIDs []string, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.VeDichQuestionSetSelected", func(state *controller.State) error {
		if slot < 1 || slot > 4 {
			return validation("slot", "contestant slot must be between 1 and 4")
		}
		if len(questionIDs) != 3 {
			return validation("question_ids", "Về đích question package must contain exactly three choices")
		}
		for i, questionID := range questionIDs {
			if !validVDChoice(questionID) {
				return validation(fmt.Sprintf("question_ids[%d]", i), "question package choice must be 20_1 through 30_3")
			}
		}
		state.VeDichContestantsState[slot-1].SelectedQuestionSet = append([]string{}, questionIDs...)
		state.SelectedContestant = slot
		return nil
	})
}

func (s *Service) SetVeDichStar(ctx context.Context, slot, question int, command Command, actor Actor) (RuntimeState, error) {
	return s.mutateController(ctx, command, actor, "Controller.VeDichStarChanged", func(state *controller.State) error {
		if slot < 1 || slot > 4 {
			return validation("slot", "contestant slot must be between 1 and 4")
		}
		if question < 0 || question > 3 {
			return validation("question", "starred question must be between 0 and 3")
		}
		state.VeDichContestantsState[slot-1].StarredOn = question
		return nil
	})
}

type VeDichVideoCommand struct {
	Command    Command `json:"command"`
	Action     string  `json:"action"`
	QuestionID string  `json:"question_id,omitempty"`
	SessionID  uint64  `json:"session_id,omitempty"`
	Volume     float64 `json:"volume,omitempty"`
	// DurationMS is reported by the operator preview with action "duration".
	DurationMS int64 `json:"duration_ms,omitempty"`
}

const maxVeDichVideoMS int64 = 60 * 60 * 1_000

// videoPosition is the media position at now on the server clock.
func videoPosition(video controller.VeDichVideoState, now int64) int64 {
	position := video.PositionMS
	if video.Status == controller.VeDichVideoPlaying && video.StartedAt > 0 {
		position += max(0, now-video.StartedAt)
	}
	if video.DurationMS > 0 {
		position = min(position, video.DurationMS)
	}
	return position
}

// frozenVideo stops the play segment clock, keeping the reached position.
func frozenVideo(video controller.VeDichVideoState, now int64) controller.VeDichVideoState {
	if video.StartedAt > 0 {
		video.PositionMS = videoPosition(video, now)
		video.StartedAt = 0
	}
	return video
}

func (s *Service) VeDichVideo(ctx context.Context, input VeDichVideoCommand, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPlayableLocked(input.Command); err != nil {
		return RuntimeState{}, err
	}
	state := s.state.Controller
	video := state.VeDichVideo
	if state.Phase != controller.PhaseVeDich || !strings.HasPrefix(string(state.Screen), "ques_ts") ||
		s.state.CurrentQuestion == nil || s.state.CurrentQuestion.ID != state.SelectedQuestionID ||
		!strings.HasPrefix(state.SelectedQuestionID, "vd.") {
		return RuntimeState{}, validation("ve_dich_video", "select an active Về đích question first")
	}
	now := time.Now().UnixMilli()
	switch input.Action {
	case "start":
		if s.state.CurrentQuestion.Type != "video" || s.state.CurrentQuestion.AssetID == "" {
			return RuntimeState{}, validation("ve_dich_video", "the selected Về đích question has no video")
		}
		if s.state.Timer.IsRunning || s.state.Timer.IsPaused {
			return RuntimeState{}, validation("timer", "reset the timer before showing the video")
		}
		duration := int64(0)
		if video.QuestionID == state.SelectedQuestionID {
			// The media length does not change between sessions of one question.
			duration = video.DurationMS
		}
		video = controller.VeDichVideoState{
			SessionID: video.SessionID + 1, QuestionID: state.SelectedQuestionID,
			Status: controller.VeDichVideoPlaying, Volume: 1, StartedAt: now, DurationMS: duration,
		}
		state.IsShowingQuestion = true
	case "pause":
		if video.Status != controller.VeDichVideoPlaying {
			return RuntimeState{}, validation("ve_dich_video.status", "only a playing video can be paused")
		}
		video = frozenVideo(video, now)
		video.Status = controller.VeDichVideoPaused
	case "resume":
		if video.Status != controller.VeDichVideoPaused {
			return RuntimeState{}, validation("ve_dich_video.status", "only a paused video can be resumed")
		}
		video.Status = controller.VeDichVideoPlaying
		video.StartedAt = now
	case "stop":
		if video.Status != controller.VeDichVideoPlaying && video.Status != controller.VeDichVideoPaused {
			return RuntimeState{}, validation("ve_dich_video.status", "there is no video to stop")
		}
		video = frozenVideo(video, now)
		video.Status = controller.VeDichVideoHolding
	case "volume":
		if video.Status == controller.VeDichVideoIdle {
			return RuntimeState{}, validation("ve_dich_video.status", "there is no video to adjust")
		}
		if input.Volume < 0 || input.Volume > 1 {
			return RuntimeState{}, validation("ve_dich_video.volume", "volume must be between zero and one")
		}
		video.Volume = input.Volume
	case "duration":
		if video.Status == controller.VeDichVideoIdle || video.Status == "" ||
			(input.SessionID != 0 && input.SessionID != video.SessionID) ||
			(input.QuestionID != "" && input.QuestionID != video.QuestionID) {
			return RuntimeState{}, stale(s.state.Version)
		}
		if input.DurationMS <= 0 || input.DurationMS > maxVeDichVideoMS {
			return RuntimeState{}, validation("duration_ms", "video duration must be between zero and one hour")
		}
		if video.DurationMS == input.DurationMS {
			return cloneRuntime(*s.state), nil
		}
		video.DurationMS = input.DurationMS
	case "finish":
		if video.QuestionID == state.SelectedQuestionID && video.SessionID == input.SessionID &&
			(input.QuestionID == "" || input.QuestionID == state.SelectedQuestionID) &&
			(video.Status == controller.VeDichVideoIdle || video.Status == "") {
			// The server already finished this session; the client fallback is a no-op.
			return cloneRuntime(*s.state), nil
		}
		if video.QuestionID != state.SelectedQuestionID || video.SessionID != input.SessionID ||
			(input.QuestionID != "" && input.QuestionID != state.SelectedQuestionID) ||
			(video.Status != controller.VeDichVideoPlaying && video.Status != controller.VeDichVideoHolding) {
			return RuntimeState{}, stale(s.state.Version)
		}
		return s.finishVeDichVideoLocked(ctx, actor, input.Command.CommandID)
	default:
		return RuntimeState{}, validation("action", "unknown Về đích video action")
	}
	state.VeDichVideo = video
	if video.Status != s.state.Controller.VeDichVideo.Status {
		s.state.BellEpoch++
	}
	s.state.Controller = state
	payload := map[string]any{
		"action": input.Action, "question_id": state.SelectedQuestionID, "session_id": video.SessionID, "volume": video.Volume,
		"started_at": video.StartedAt, "position_ms": video.PositionMS, "duration_ms": video.DurationMS,
	}
	updated, err := s.persistLocked(ctx, "Controller.VeDichVideoChanged", payload, actor, input.Command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

// finishVeDichVideoLocked ends the takeover and starts the question timer.
// Callers have validated that the current video session is still active.
func (s *Service) finishVeDichVideoLocked(ctx context.Context, actor Actor, commandID string) (RuntimeState, error) {
	state := s.state.Controller
	video := state.VeDichVideo
	duration, err := questionTimerDuration(state, s.state.CurrentQuestion)
	if err != nil {
		return RuntimeState{}, err
	}
	now := time.Now().UnixMilli()
	video = frozenVideo(video, now)
	video.Status = controller.VeDichVideoIdle
	state.VeDichVideo = video
	s.state.Controller = state
	s.state.Timer = TimerState{IsRunning: true, DurationMS: duration, RemainingMS: duration, StartedAt: now, Kind: TimerKindQuestion}
	s.state.InputEpoch++
	s.state.BellEpoch++
	s.resetInputsLocked(false)
	s.state.Controller.AnswerStatus = []bool{true, true, true, true}
	updated, err := s.persistLocked(ctx, "Controller.VeDichVideoFinished", map[string]any{
		"question_id": state.SelectedQuestionID, "session_id": video.SessionID, "timer": s.state.Timer,
	}, actor, commandID)
	if err == nil {
		s.runTimerLocked(duration)
		s.broadcastLocked()
	}
	return updated, err
}

// syncVideoScheduleLocked (re)arms the server-side video finish so it matches
// the current state: end of media + final-frame hold while playing, or the
// hold after an operator stop. It is idempotent and runs after every broadcast.
func (s *Service) syncVideoScheduleLocked() {
	if s.state == nil {
		s.cancelVideoLocked()
		return
	}
	video := s.state.Controller.VeDichVideo
	now := time.Now().UnixMilli()
	fireAt := int64(0)
	if s.state.Status == StatusPlaying && s.state.Controller.Phase == controller.PhaseVeDich {
		switch video.Status {
		case controller.VeDichVideoPlaying:
			if video.DurationMS > 0 && video.StartedAt > 0 {
				fireAt = video.StartedAt + max(0, video.DurationMS-video.PositionMS) + veDichVideoHoldMS
			}
		case controller.VeDichVideoHolding:
			if s.videoHoldSession != video.SessionID {
				s.videoHoldSession = video.SessionID
				s.videoHoldUntil = now + veDichVideoHoldMS
			}
			fireAt = s.videoHoldUntil
		}
	}
	if video.Status != controller.VeDichVideoHolding || s.state.Status != StatusPlaying {
		// A paused match restarts the full hold when play resumes.
		s.videoHoldSession = 0
	}
	if fireAt == 0 {
		s.cancelVideoLocked()
		return
	}
	key := fmt.Sprintf("%s:%d:%d:%d", s.state.MatchID, s.state.Epoch, video.SessionID, fireAt)
	if key == s.videoScheduleKey && s.videoCancel != nil {
		return
	}
	s.cancelVideoLocked()
	s.videoScheduleKey = key
	generation := s.videoGeneration
	timerCtx, cancel := context.WithCancel(context.Background())
	s.videoCancel = cancel
	matchID, epoch, session := s.state.MatchID, s.state.Epoch, video.SessionID
	go func() {
		select {
		case <-time.After(time.Duration(max(0, fireAt-now)) * time.Millisecond):
		case <-timerCtx.Done():
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.state == nil || s.state.MatchID != matchID || s.state.Epoch != epoch || s.videoGeneration != generation ||
			s.state.Status != StatusPlaying {
			return
		}
		current := s.state.Controller.VeDichVideo
		if current.SessionID != session || current.QuestionID != s.state.Controller.SelectedQuestionID ||
			(current.Status != controller.VeDichVideoPlaying && current.Status != controller.VeDichVideoHolding) {
			return
		}
		s.videoCancel = nil
		s.videoScheduleKey = ""
		_, _ = s.finishVeDichVideoLocked(context.Background(), Actor{}, "")
	}()
}

func (s *Service) cancelVideoLocked() {
	s.videoGeneration++
	s.videoScheduleKey = ""
	if s.videoCancel != nil {
		s.videoCancel()
		s.videoCancel = nil
	}
}

func (s *Service) SubmitAnswer(ctx context.Context, slot int, answer string, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkInputLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if (s.state.Controller.Phase != controller.PhaseVCNV && s.state.Controller.Phase != controller.PhaseTangToc) ||
		s.state.Controller.Screen != controller.ScreenQues || !s.state.Controller.IsShowingQuestion || s.state.CurrentQuestion == nil ||
		!s.state.Timer.IsRunning || s.state.Timer.IsPaused || timerRemaining(s.state.Timer, time.Now().UnixMilli()) <= 0 {
		return RuntimeState{}, &opsocket.Error{Code: "ANSWER_WINDOW_CLOSED", Message: "the question is not accepting answers"}
	}
	contestant, err := s.contestantLocked(slot)
	if err != nil {
		return RuntimeState{}, err
	}
	if contestant.Locked {
		return RuntimeState{}, &opsocket.Error{Code: "INPUT_LOCKED", Message: "contestant input is locked"}
	}
	answer = strings.TrimSpace(answer)
	if answer == "" || utf8.RuneCountInString(answer) > 1000 {
		return RuntimeState{}, validation("answer", "answer is required and must not exceed 1000 characters")
	}
	// The last answer inside the window counts; resubmitting also moves the
	// recorded time, which Tăng tốc ranks on. Only an operator lock blocks input.
	contestant.Answer = answer
	contestant.TimeMS = timerElapsed(s.state.Timer, time.Now().UnixMilli())
	updated, err := s.persistLocked(ctx, "Contestant.AnswerSubmitted", map[string]any{"slot": slot, "answer": contestant.Answer, "time_ms": contestant.TimeMS}, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) Ring(ctx context.Context, slot int, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkBellInputLocked(command); err != nil {
		return RuntimeState{}, err
	}
	now := time.Now().UnixMilli()
	if !bellAvailable(*s.state, now) {
		return RuntimeState{}, &opsocket.Error{Code: "BELL_CLOSED", Message: "the bell is not accepting input"}
	}
	contestant, err := s.contestantLocked(slot)
	if err != nil {
		return RuntimeState{}, err
	}
	if contestant.BellLocked || contestant.HasRung {
		return RuntimeState{}, &opsocket.Error{Code: "INPUT_LOCKED", Message: "contestant cannot ring"}
	}
	if s.state.Controller.Phase == controller.PhaseVeDich && slot == veDichQuestionOwner(*s.state) {
		return RuntimeState{}, &opsocket.Error{Code: "INPUT_LOCKED", Message: "the question owner cannot steal"}
	}
	bellTime := int64(0)
	if s.state.Timer.IsRunning && !s.state.Timer.IsPaused {
		bellTime = timerElapsed(s.state.Timer, now)
	}
	contestant.HasRung = true
	// A VCNV contestant can ring after submitting a typed answer. That answer's
	// submission time remains private and unchanged; the event has the bell time.
	if contestant.Answer == "" {
		contestant.TimeMS = bellTime
	}
	updated, err := s.persistLocked(ctx, "Contestant.Rang", map[string]any{"slot": slot, "time_ms": bellTime}, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) AdjustScore(ctx context.Context, slot, delta int, reason string, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusCompleted || s.state.Status == StatusArchived {
		return RuntimeState{}, validation("match", "completed matches require a result amendment")
	}
	if delta < -1000 || delta > 1000 {
		return RuntimeState{}, validation("delta", "score adjustment is out of range")
	}
	contestant, err := s.contestantLocked(slot)
	if err != nil {
		return RuntimeState{}, err
	}
	contestant.Score += delta
	updated, err := s.persistLocked(ctx, "Match.ScoreAdjusted", map[string]any{"slot": slot, "delta": delta, "score": contestant.Score, "reason": strings.TrimSpace(reason)}, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) AdjustScores(ctx context.Context, adjustments []ScoreAdjustment, reason string, command Command, actor Actor, sources ...ScoreAdjustmentSource) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusCompleted || s.state.Status == StatusArchived {
		return RuntimeState{}, validation("match", "completed matches require a result amendment")
	}
	if len(adjustments) == 0 || len(adjustments) > len(s.state.Contestants) {
		return RuntimeState{}, validation("adjustments", "one score adjustment per contestant is required")
	}
	seen := make(map[int]bool, len(adjustments))
	for _, adjustment := range adjustments {
		if adjustment.Delta < -1000 || adjustment.Delta > 1000 {
			return RuntimeState{}, validation("delta", "score adjustment is out of range")
		}
		if adjustment.Delta == 0 {
			return RuntimeState{}, validation("delta", "score adjustment cannot be zero")
		}
		if seen[adjustment.Slot] {
			return RuntimeState{}, validation("slot", "each contestant can only be adjusted once")
		}
		seen[adjustment.Slot] = true
		if _, err := s.contestantLocked(adjustment.Slot); err != nil {
			return RuntimeState{}, err
		}
	}
	if err := s.validateScoreSourcesLocked(ctx, reason, sources); err != nil {
		return RuntimeState{}, err
	}

	previous := cloneRuntime(*s.state)
	result := make([]map[string]int, 0, len(adjustments))
	for _, adjustment := range adjustments {
		contestant, _ := s.contestantLocked(adjustment.Slot)
		contestant.Score += adjustment.Delta
		result = append(result, map[string]int{"slot": adjustment.Slot, "delta": adjustment.Delta, "score": contestant.Score})
	}
	payload := map[string]any{"adjustments": result, "reason": strings.TrimSpace(reason)}
	if command.CommandID != "" {
		payload["score_save_id"] = command.CommandID
	}
	if len(sources) > 0 {
		payload["sources"] = sources
	}
	updated, err := s.persistLocked(ctx, "Match.ScoresAdjusted", payload, actor, command.CommandID)
	if err == nil && updated.Version != previous.Version+1 {
		s.state = &previous
		return RuntimeState{}, stale(previous.Version)
	}
	if err != nil {
		s.state = &previous
	} else {
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) SetContestantLock(ctx context.Context, slot int, locked bool, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if s.state.Status == StatusCompleted || s.state.Status == StatusArchived {
		return RuntimeState{}, validation("match", "completed matches are immutable")
	}
	contestant, err := s.contestantLocked(slot)
	if err != nil {
		return RuntimeState{}, err
	}
	contestant.Locked = locked
	contestant.BellLocked = locked
	if !locked {
		contestant.Answer = ""
		contestant.TimeMS = 0
		// VCNV retains the operator's explicit retry override. KD/VD's one
		// claimed slot belongs to the question and cannot be unlocked away.
		if s.state.Controller.Phase == controller.PhaseVCNV {
			contestant.HasRung = false
		}
	}
	updated, err := s.persistLocked(ctx, "Contestant.LockChanged", map[string]any{"slot": slot, "locked": locked}, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) contestantLocked(slot int) (*ContestantState, error) {
	for i := range s.state.Contestants {
		if s.state.Contestants[i].Slot == slot {
			return &s.state.Contestants[i], nil
		}
	}
	return nil, validation("slot", "contestant slot not found")
}
func (s *Service) Controller(ctx context.Context) controller.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return controller.DefaultState()
	}
	normalizeBell(s.state, time.Now().UnixMilli())
	return cloneController(s.state.Controller)
}

type TimerStart struct {
	Command Command `json:"command"`
	Steal   bool    `json:"steal,omitempty"`
	// Obstacle starts the 15 s VCNV board timer (main_vcnv/image_vcnv); it
	// needs no selected question and leaves contestant inputs untouched.
	Obstacle bool `json:"obstacle,omitempty"`
	// Deprecated: accepted for cached clients, but game rules determine the duration.
	DurationMS int64 `json:"duration_ms,omitempty"`
}

// questionTimerDuration uses the selected snapshot, not the latest question-set
// revision, so editing a question cannot change its already selected duration.
func questionTimerDuration(state controller.State, question *QuestionSnapshot) (int64, error) {
	if state.SelectedQuestionID == "" || question == nil || question.ID != state.SelectedQuestionID {
		return 0, validation("question", "select a question from the active phase before starting the timer")
	}
	parts := strings.Split(question.ID, ".")
	switch {
	case state.Phase == controller.PhaseKhoiDong &&
		(len(parts) == 2 && parts[0] == "kd_chung" || len(parts) == 3 && parts[0] == "kd_rieng"):
		return 3_000, nil
	case state.Phase == controller.PhaseVCNV && len(parts) == 3 && parts[0] == "vcnv" && parts[1] == "rows":
		return 15_000, nil
	case state.Phase == controller.PhaseTangToc && len(parts) == 2 && parts[0] == "tt",
		state.Phase == controller.PhaseVCNV && question.ID == "vcnv.center":
		seconds := question.Time
		if seconds > 600 {
			return 0, validation("question.time", "question duration must not exceed 10 minutes")
		}
		if seconds <= 0 {
			// Legacy: the VCNV centre is a 15 s row-style question.
			seconds = 30
			if question.ID == "vcnv.center" {
				seconds = 15
			}
		}
		return int64(seconds) * 1_000, nil
	case state.Phase == controller.PhaseVeDich && len(parts) == 4 && parts[0] == "vd":
		switch parts[2] {
		case "20":
			return 15_000, nil
		case "30":
			return 20_000, nil
		}
	}
	return 0, validation("question", "select a question from the active phase before starting the timer")
}

func (s *Service) StartTimer(ctx context.Context, input TimerStart, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPlayableLocked(input.Command); err != nil {
		return RuntimeState{}, err
	}
	if input.Obstacle {
		return s.startObstacleTimerLocked(ctx, input, actor)
	}
	if s.state.Controller.VeDichVideo.Status == controller.VeDichVideoPlaying ||
		s.state.Controller.VeDichVideo.Status == controller.VeDichVideoPaused ||
		s.state.Controller.VeDichVideo.Status == controller.VeDichVideoHolding {
		return RuntimeState{}, validation("ve_dich_video", "the Về đích video must finish before the timer starts")
	}
	duration, err := questionTimerDuration(s.state.Controller, s.state.CurrentQuestion)
	if err != nil {
		return RuntimeState{}, err
	}
	if input.Steal {
		if !activeVeDichQuestion(*s.state) {
			return RuntimeState{}, validation("steal", "reveal the selected question on its owner's Về đích screen first")
		}
		if hasBellClaim(*s.state) || s.state.Timer.DurationMS == stealDurationMS {
			return RuntimeState{}, validation("steal", "this question's steal has already started or been claimed")
		}
		// An explicit steal declaration replaces even an unfinished owner timer.
		duration = stealDurationMS
	} else if s.state.Timer.IsRunning {
		return RuntimeState{}, &opsocket.Error{Code: "TIMER_RUNNING", Message: "timer is already running"}
	}
	kind := TimerKindQuestion
	if input.Steal {
		kind = TimerKindSteal
	}
	now := time.Now()
	s.state.Timer = TimerState{IsRunning: true, DurationMS: duration, RemainingMS: duration, StartedAt: now.UnixMilli(), Kind: kind}
	s.state.InputEpoch++
	s.resetInputsLocked(false)
	if s.state.Controller.Phase == controller.PhaseVeDich {
		s.state.BellEpoch++
	}
	s.state.Controller.AnswerStatus = []bool{true, true, true, true}
	updated, err := s.persistLocked(ctx, "Timer.Started", s.state.Timer, actor, input.Command.CommandID)
	if err == nil {
		s.runTimerLocked(duration)
		s.broadcastLocked()
	}
	return updated, err
}
func (s *Service) startObstacleTimerLocked(ctx context.Context, input TimerStart, actor Actor) (RuntimeState, error) {
	state := s.state.Controller
	if state.Phase != controller.PhaseVCNV || (state.Screen != controller.ScreenMainVCNV && state.Screen != controller.ScreenImageVCNV) {
		return RuntimeState{}, validation("obstacle", "the obstacle timer runs on the VCNV board or image screen")
	}
	if s.state.Timer.IsRunning && !s.state.Timer.IsPaused {
		return RuntimeState{}, &opsocket.Error{Code: "TIMER_RUNNING", Message: "timer is already running"}
	}
	s.state.Timer = TimerState{IsRunning: true, DurationMS: obstacleDurationMS, RemainingMS: obstacleDurationMS, StartedAt: time.Now().UnixMilli(), Kind: TimerKindObstacle}
	updated, err := s.persistLocked(ctx, "Timer.Started", s.state.Timer, actor, input.Command.CommandID)
	if err == nil {
		s.runTimerLocked(obstacleDurationMS)
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) PauseTimer(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if !s.state.Timer.IsRunning || s.state.Timer.IsPaused {
		return RuntimeState{}, &opsocket.Error{Code: "TIMER_NOT_RUNNING", Message: "timer is not running"}
	}
	s.pauseTimerLocked()
	if s.state.Controller.Phase == controller.PhaseVeDich {
		s.state.BellEpoch++
	}
	event := "Timer.Paused"
	if s.state.Timer.RemainingMS == 0 {
		event = "Timer.Finished"
	}
	updated, err := s.persistLocked(ctx, event, s.state.Timer, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}
func (s *Service) ResumeTimer(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkPlayableLocked(command); err != nil {
		return RuntimeState{}, err
	}
	if !s.state.Timer.IsPaused {
		return RuntimeState{}, &opsocket.Error{Code: "TIMER_NOT_PAUSED", Message: "timer is not paused"}
	}
	if s.state.Timer.RemainingMS <= 0 {
		return RuntimeState{}, &opsocket.Error{Code: "TIMER_FINISHED", Message: "the timer has no remaining time"}
	}
	s.state.Timer.IsPaused = false
	s.state.Timer.IsRunning = true
	s.state.Timer.StartedAt = time.Now().UnixMilli()
	if s.state.Controller.Phase == controller.PhaseVeDich {
		s.state.BellEpoch++
	}
	// DurationMS and ElapsedMS retain the whole question's active duration.
	updated, err := s.persistLocked(ctx, "Timer.Resumed", s.state.Timer, actor, command.CommandID)
	if err == nil {
		s.runTimerLocked(s.state.Timer.RemainingMS)
		s.broadcastLocked()
	}
	return updated, err
}
func (s *Service) ResetTimer(ctx context.Context, command Command, actor Actor) (RuntimeState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkCommandLocked(command); err != nil {
		return RuntimeState{}, err
	}
	s.cancelTimerLocked()
	s.state.Timer = TimerState{}
	s.state.InputEpoch++
	if s.state.Controller.Phase == controller.PhaseVeDich {
		s.state.BellEpoch++
	}
	updated, err := s.persistLocked(ctx, "Timer.Reset", nil, actor, command.CommandID)
	if err == nil {
		s.broadcastLocked()
	}
	return updated, err
}

func (s *Service) runTimerLocked(durationMS int64) {
	s.cancelTimerLocked()
	generation := s.timerGeneration
	ctx, cancel := context.WithCancel(context.Background())
	s.timerCancel = cancel
	matchID := s.state.MatchID
	epoch := s.state.Epoch
	go func() {
		select {
		case <-time.After(time.Duration(durationMS) * time.Millisecond):
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.state == nil || s.state.MatchID != matchID || s.state.Epoch != epoch || s.timerGeneration != generation {
				return
			}
			s.state.Timer.IsRunning = false
			s.state.Timer.IsPaused = false
			s.state.Timer.RemainingMS = 0
			s.state.Timer.ElapsedMS = s.state.Timer.DurationMS
			s.state.Timer.StartedAt = 0
			if s.state.Controller.Phase == controller.PhaseVeDich {
				s.state.BellEpoch++
			}
			if _, err := s.persistLocked(context.Background(), "Timer.Finished", nil, Actor{}, ""); err != nil {
				// persistLocked restores the last durable state on failure. Keep
				// the in-memory timer from remaining permanently "running"
				// after its worker has already exited.
				s.state.Timer.IsRunning = false
				s.state.Timer.IsPaused = false
				s.state.Timer.RemainingMS = 0
				s.state.Timer.ElapsedMS = s.state.Timer.DurationMS
				s.state.Timer.StartedAt = 0
				s.timerCancel = nil
				s.broadcastLocked()
				return
			}
			s.broadcastLocked()
		case <-ctx.Done():
		}
	}()
}
func (s *Service) pauseTimerLocked() {
	if s.state == nil {
		return
	}
	if s.state.Timer.IsRunning && !s.state.Timer.IsPaused {
		s.state.Timer = frozenTimer(s.state.Timer, time.Now().UnixMilli())
		s.state.Timer.IsRunning = s.state.Timer.RemainingMS > 0
		s.state.Timer.IsPaused = s.state.Timer.RemainingMS > 0
	}
	s.cancelTimerLocked()
}

// pausedTimer freezes a running timer at now and leaves it resumable, the
// same shape PauseTimer produces. Idle and finished timers are unchanged.
func pausedTimer(timer TimerState, now int64) TimerState {
	if !timer.IsRunning || timer.IsPaused {
		return timer
	}
	timer = frozenTimer(timer, now)
	timer.IsRunning = timer.RemainingMS > 0
	timer.IsPaused = timer.RemainingMS > 0
	return timer
}

// Shutdown pauses a running timer and video so a restart resumes them with the
// time that was left, then persists. Call it once while the server stops.
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelVideoLocked()
	if s.state == nil {
		s.cancelTimerLocked()
		return nil
	}
	now := time.Now().UnixMilli()
	timerActive := s.state.Timer.IsRunning && !s.state.Timer.IsPaused
	videoActive := s.state.Controller.VeDichVideo.StartedAt > 0
	s.cancelTimerLocked()
	if !timerActive && !videoActive {
		return nil
	}
	s.state.Timer = pausedTimer(s.state.Timer, now)
	s.state.Controller.VeDichVideo = frozenVideo(s.state.Controller.VeDichVideo, now)
	_, err := s.persistLocked(ctx, "Match.ShutdownPaused", map[string]any{"timer": s.state.Timer, "ve_dich_video": s.state.Controller.VeDichVideo}, Actor{}, "")
	return err
}

func (s *Service) cancelTimerLocked() {
	s.timerGeneration++
	if s.timerCancel != nil {
		s.timerCancel()
		s.timerCancel = nil
	}
}

func timerRemaining(timer TimerState, now int64) int64 {
	if timer.IsRunning && !timer.IsPaused {
		return max(0, timer.RemainingMS-max(0, now-timer.StartedAt))
	}
	return timer.RemainingMS
}

func timerElapsed(timer TimerState, now int64) int64 {
	elapsed := timer.ElapsedMS
	if timer.IsRunning && !timer.IsPaused {
		elapsed += max(0, now-timer.StartedAt)
	}
	return min(timer.DurationMS, elapsed)
}

func frozenTimer(timer TimerState, now int64) TimerState {
	timer.ElapsedMS = timerElapsed(timer, now)
	timer.RemainingMS = timerRemaining(timer, now)
	timer.IsRunning = false
	timer.StartedAt = 0
	return timer
}

func (s *Service) checkInputLocked(command Command) error {
	// An unrelated score/answer update must not invalidate another contestant's
	// input. Bind it to this match and input window, not the global version.
	command.ExpectedVersion = 0
	if err := s.checkPlayableLocked(command); err != nil {
		return err
	}
	if command.MatchID == "" || command.ExpectedEpoch == 0 ||
		command.ExpectedInputEpoch == 0 || command.ExpectedInputEpoch != s.state.InputEpoch {
		return stale(s.state.Version)
	}
	return nil
}
func (s *Service) checkBellInputLocked(command Command) error {
	command.ExpectedVersion = 0
	if err := s.checkPlayableLocked(command); err != nil {
		return err
	}
	if command.MatchID == "" || command.ExpectedEpoch == 0 ||
		command.ExpectedBellEpoch == 0 || command.ExpectedBellEpoch != s.state.BellEpoch {
		return stale(s.state.Version)
	}
	return nil
}

func (s *Service) checkPlayableLocked(command Command) error {
	if err := s.checkCommandLocked(command); err != nil {
		return err
	}
	if s.state.Status != StatusPlaying {
		return &opsocket.Error{Code: "MATCH_NOT_PLAYING", Message: "the selected match is not playing", Version: s.state.Version}
	}
	return nil
}
func (s *Service) checkCommandLocked(command Command) error {
	if s.state == nil {
		return &opsocket.Error{Code: "NO_MATCH", Message: "no match is selected"}
	}
	if command.MatchID != "" && command.MatchID != s.state.MatchID {
		return stale(s.state.Version)
	}
	if command.ExpectedEpoch != 0 && command.ExpectedEpoch != s.state.Epoch {
		return stale(s.state.Version)
	}
	if command.ExpectedVersion != 0 && command.ExpectedVersion != s.state.Version {
		return stale(s.state.Version)
	}
	return nil
}
func stale(version int64) error {
	return &opsocket.Error{Code: "STALE_RUNTIME", Message: "the match state changed; refresh and try again", Retryable: true, Version: version}
}
func (s *Service) persistLocked(ctx context.Context, event string, payload any, actor Actor, commandID string) (RuntimeState, error) {
	normalizeBell(s.state, time.Now().UnixMilli())
	updated, err := s.repository.SaveRuntime(ctx, *s.state, event, payload, actor, commandID)
	if err != nil {
		if restored, restoreErr := s.repository.LoadRuntime(ctx, s.state.MatchID, s.state.Epoch); restoreErr == nil {
			s.state = &restored
			normalizeBell(s.state, time.Now().UnixMilli())
		}
		return RuntimeState{}, err
	}
	s.state = &updated
	return cloneRuntime(updated), nil
}
func (s *Service) broadcastLocked() {
	if s.state == nil {
		return
	}
	s.syncVideoScheduleLocked()
	if s.publish == nil {
		return
	}
	normalizeBell(s.state, time.Now().UnixMilli())
	s.publish("role:admin", "Match.Updated", cloneRuntime(*s.state))
	s.publish("public", "Match.PublicUpdated", filteredState(*s.state, opsocket.Principal{}))
	for slot := 1; slot <= 4; slot++ {
		s.publish(fmt.Sprintf("contestant:%d", slot), "Match.ContestantUpdated", filteredState(*s.state, opsocket.Principal{Claims: map[string]string{"slot": fmt.Sprint(slot)}}))
	}
	s.notifyLocked()
}

// SetVerdictSound installs the Khởi động verdict cue. It runs after the judgment is committed
// and before the next state is broadcast, so the sound is never delayed by the question change.
func (s *Service) SetVerdictSound(play func(verdict string, actor Actor) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verdictSound = play
}

// SetObserver installs the state-change observer (sound cues). Call before serving.
func (s *Service) SetObserver(observer Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observer = observer
	if s.state != nil {
		current := cloneRuntime(*s.state)
		s.lastObserved = &current
	}
}

func (s *Service) notifyLocked() {
	if s.observer == nil || s.state == nil {
		return
	}
	current := cloneRuntime(*s.state)
	previous := RuntimeState{}
	if s.lastObserved != nil {
		previous = *s.lastObserved
	}
	s.lastObserved = &current
	s.observer(previous, cloneRuntime(current))
}

func (s *Service) broadcastEmptyLocked() {
	s.cancelVideoLocked()
	if s.publish == nil {
		return
	}
	empty := RuntimeState{}
	s.publish("role:admin", "Match.Updated", empty)
	s.publish("public", "Match.PublicUpdated", empty)
	for slot := 1; slot <= 4; slot++ {
		s.publish(fmt.Sprintf("contestant:%d", slot), "Match.ContestantUpdated", empty)
	}
}

// FilterRuntime applies the same role policy to mutation replies as broadcasts.
func FilterRuntime(state RuntimeState, principal opsocket.Principal) RuntimeState {
	return filteredState(state, principal)
}
func filteredState(state RuntimeState, principal opsocket.Principal) RuntimeState {
	result := cloneRuntime(state)
	normalizeBell(&result, time.Now().UnixMilli())
	if principal.HasRole(string(auth.RoleAdmin)) {
		return result
	}
	if question := result.CurrentQuestion; question != nil {
		ttSolve := result.Controller.Phase == controller.PhaseTangToc && result.Controller.Screen == controller.ScreenSolve
		if !result.Controller.IsShowingQuestion && !ttSolve {
			// Hidden question: enough for the frame (number, type, media) only.
			result.CurrentQuestion = &QuestionSnapshot{ID: question.ID, Type: question.Type, AssetID: question.AssetID, Time: question.Time, CharCount: question.CharCount}
		} else {
			if !controller.AnswerVisible(result.Controller) {
				question.Answer = ""
				question.Keyword = ""
			}
			if !ttSolve {
				question.ExplanationAssetID = ""
			}
		}
	}
	slot := 0
	if principal.Claims != nil {
		fmt.Sscan(principal.Claims["slot"], &slot)
	}
	for i := range result.Contestants {
		if result.Contestants[i].Slot != slot && !controller.ContestantAnswersVisible(result.Controller) {
			result.Contestants[i].Answer = ""
			result.Contestants[i].TimeMS = 0
		}
	}
	return result
}
func cloneRuntime(state RuntimeState) RuntimeState {
	raw, _ := json.Marshal(state)
	var result RuntimeState
	_ = json.Unmarshal(raw, &result)
	return result
}
func contestantStates(contestants []Contestant, existing []ContestantState) []ContestantState {
	bySlot := map[int]ContestantState{}
	for _, state := range existing {
		bySlot[state.Slot] = state
	}
	result := make([]ContestantState, len(contestants))
	for i, c := range contestants {
		state := bySlot[c.Slot]
		state.Slot = c.Slot
		state.Name = c.Name
		state.Class = c.Class
		result[i] = state
	}
	return result
}

func (s *Service) validatedMatch(input CreateMatchInput) (Match, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Stage = strings.TrimSpace(input.Stage)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 120 {
		return Match{}, validation("name", "match name is required and must not exceed 120 characters")
	}
	if input.Season < 1 || input.Season > 99 {
		return Match{}, validation("season", "season must be between 1 and 99")
	}
	if input.Stage == "" || utf8.RuneCountInString(input.Stage) > 80 {
		return Match{}, validation("stage", "stage is required and must not exceed 80 characters")
	}
	if len(input.Contestants) != 4 {
		return Match{}, validation("contestants", "exactly four contestants are required")
	}
	seen := map[int]bool{}
	for i := range input.Contestants {
		contestant := &input.Contestants[i]
		if contestant.Slot == 0 {
			contestant.Slot = i + 1
		}
		user, ok := s.fixedUsers[contestant.Slot]
		if !ok || seen[contestant.Slot] {
			return Match{}, validation("contestants", "contestant slots must be unique values from 1 to 4")
		}
		seen[contestant.Slot] = true
		contestant.UserID = user.ID
		contestant.Name = strings.TrimSpace(contestant.Name)
		contestant.Class = strings.TrimSpace(contestant.Class)
		if contestant.Name == "" {
			return Match{}, validation("contestants", "each contestant name is required")
		}
	}
	return Match{Name: input.Name, Season: input.Season, Stage: input.Stage, QuestionSetID: input.QuestionSetID, Contestants: input.Contestants, Result: json.RawMessage(`{}`)}, nil
}

func (s *Service) ListQuestionSets(ctx context.Context, search string) ([]QuestionSet, error) {
	return s.repository.ListQuestionSets(ctx, strings.TrimSpace(search))
}
func (s *Service) GetQuestionSet(ctx context.Context, id string) (QuestionSet, QuestionSetRevision, error) {
	return s.repository.GetQuestionSet(ctx, id)
}
func (s *Service) GetQuestionSetRevision(ctx context.Context, id string) (QuestionSetRevision, error) {
	revision, err := s.repository.GetQuestionSetRevision(ctx, id)
	if err != nil {
		return QuestionSetRevision{}, notFound("question set revision")
	}
	return revision, nil
}

func (s *Service) CurrentVCNV(ctx context.Context) (VCNVDisplay, error) {
	s.mu.Lock()
	revisionID := ""
	rowStates := make([]string, 4)
	hasSolved := false
	if s.state != nil {
		revisionID = s.state.QuestionRevisionID
		hasSolved = s.state.Controller.VCNVState.HasSolved
		copy(rowStates, s.state.Controller.VCNVState.RowStates)
	}
	s.mu.Unlock()
	if revisionID == "" {
		return VCNVDisplay{RowLengths: make([]int, 4), RowAnswers: make([]string, 4)}, nil
	}
	revision, err := s.repository.GetQuestionSetRevision(ctx, revisionID)
	if err != nil {
		return VCNVDisplay{}, notFound("question set revision")
	}
	var data struct {
		VCNV struct {
			VCNVDisplay
			Rows []json.RawMessage `json:"rows"`
		} `json:"vcnv"`
	}
	if err := json.Unmarshal(revision.Data, &data); err != nil {
		return VCNVDisplay{}, validation("question set", "the pinned question revision is invalid")
	}
	display := data.VCNV.VCNVDisplay
	display.RowLengths = make([]int, 4)
	display.RowAnswers = make([]string, 4)
	obstacle := strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || character == '-' {
			return -1
		}
		return character
	}, display.Obstacle)
	display.ObstacleLength = utf8.RuneCountInString(obstacle)
	if !hasSolved {
		display.Obstacle = ""
	}
	for i, row := range data.VCNV.Rows {
		if i >= len(display.RowLengths) {
			break
		}
		var keyword struct {
			Keyword string `json:"keyword"`
		}
		if err := json.Unmarshal(row, &keyword); err == nil {
			answer := strings.Map(func(character rune) rune {
				if unicode.IsSpace(character) || character == '-' {
					return -1
				}
				return character
			}, keyword.Keyword)
			display.RowLengths[i] = utf8.RuneCountInString(answer)
			if i < len(rowStates) && rowStates[i] == "c" {
				display.RowAnswers[i] = strings.ToUpper(answer)
			}
		}
	}
	return display, nil
}
func (s *Service) SaveQuestionSet(ctx context.Context, id string, input SaveQuestionSetInput, actor Actor) (QuestionSet, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)
	normalized, normalizeErr := normalizeQuestionData(input.Data)
	if normalizeErr != nil {
		return QuestionSet{}, validation("data", normalizeErr.Error())
	}
	input.Data = normalized
	if !validQuestionData(input.Data) {
		return QuestionSet{}, validation("data", "question data must contain kd_chung, kd_rieng, vcnv (four rows and center question), tt, and vd")
	}
	if input.Name == "" {
		return QuestionSet{}, validation("name", "question set name is required")
	}
	set := QuestionSet{ID: id, Name: input.Name, Category: input.Category}
	revision := QuestionSetRevision{ID: uuid.NewString(), QuestionSetID: id, Data: append(json.RawMessage(nil), input.Data...), CreatedBy: actor.UserID}

	var saved QuestionSet
	var err error
	if id == "" {
		set.ID = uuid.NewString()
		revision.QuestionSetID = set.ID
		saved, err = s.repository.CreateQuestionSet(ctx, set, revision, actor)
	} else {
		saved, err = s.repository.SaveQuestionSet(ctx, set, revision, input.Version, actor)
	}
	if err != nil {
		return QuestionSet{}, conflict(err)
	}
	applied, err := s.repository.PinSelectedMatchRevision(ctx, saved.ID, saved.CurrentRevisionID, "auto-applied question set revision", actor)
	if err != nil {
		return QuestionSet{}, err
	}
	if applied {
		s.mu.Lock()
		if s.state != nil && s.state.QuestionSetID == saved.ID {
			s.state.QuestionRevisionID = saved.CurrentRevisionID
			s.state.Version++
			if _, persistErr := s.persistLocked(ctx, "Match.QuestionRevisionChanged", map[string]any{"question_set_id": saved.ID, "revision_id": saved.CurrentRevisionID}, actor, ""); persistErr != nil {
				s.mu.Unlock()
				return QuestionSet{}, persistErr
			}
			s.broadcastLocked()
		}
		s.mu.Unlock()
	}
	return saved, nil
}
func validVDChoice(value string) bool {
	if len(value) != 4 || (value[:3] != "20_" && value[:3] != "30_") {
		return false
	}
	return value[3] >= '1' && value[3] <= '3'
}
func (s *Service) ArchiveQuestionSet(ctx context.Context, id string, version int64, actor Actor) error {
	if err := s.repository.ArchiveQuestionSet(ctx, id, version, actor); err != nil {
		return conflict(err)
	}
	return nil
}

func normalizeQuestionData(raw json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(raw) {
		return nil, errors.New("question data is not valid JSON")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, errors.New("question data must be an object")
	}
	if _, ok := root["kd_chung"]; ok {
		return raw, nil
	}
	for _, key := range []string{"kd", "vcnv", "tt", "vd"} {
		if _, ok := root[key]; !ok {
			return nil, fmt.Errorf("legacy question data is missing %s", key)
		}
	}
	var kd map[string]json.RawMessage
	if err := json.Unmarshal(root["kd"], &kd); err != nil {
		return nil, errors.New("legacy kd round must be an object")
	}
	chung, ok := kd["chung"]
	if !ok {
		return nil, errors.New("legacy kd round is missing chung")
	}
	rieng, ok := kd["rieng"]
	if !ok {
		return nil, errors.New("legacy kd round is missing rieng")
	}
	var vcnv map[string]json.RawMessage
	if err := json.Unmarshal(root["vcnv"], &vcnv); err != nil {
		return nil, errors.New("legacy vcnv round must be an object")
	}
	rows, ok := vcnv["rows"]
	if !ok {
		return nil, errors.New("legacy vcnv round is missing rows")
	}
	center := vcnv["center"]
	if len(center) == 0 {
		center = vcnv["center_ques"]
	}
	if len(center) == 0 {
		return nil, errors.New("legacy vcnv round is missing center_ques")
	}
	var centerValue any
	if err := json.Unmarshal(center, &centerValue); err != nil {
		return nil, errors.New("legacy vcnv center question is invalid")
	}
	if _, isString := centerValue.(string); isString {
		centerValue = map[string]any{"content": centerValue, "type": "text"}
	}
	center, err := json.Marshal(centerValue)
	if err != nil {
		return nil, errors.New("legacy vcnv center question is invalid")
	}
	obstacle := vcnv["obstacle"]
	if len(obstacle) == 0 {
		obstacle = json.RawMessage(`""`)
	}
	obstacleAsset := vcnv["obstacle_asset_id"]
	if len(obstacleAsset) == 0 {
		obstacleAsset = vcnv["image"]
	}
	if len(obstacleAsset) == 0 {
		obstacleAsset = json.RawMessage(`""`)
	}
	var tt []map[string]any
	if err := json.Unmarshal(root["tt"], &tt); err != nil {
		return nil, errors.New("legacy tt round must be an array")
	}
	for _, question := range tt {
		if value, exists := question["type"]; !exists || strings.TrimSpace(fmt.Sprint(value)) == "" || value == nil {
			question["type"] = "text"
		}
	}
	ttRaw, err := json.Marshal(tt)
	if err != nil {
		return nil, errors.New("legacy tt round is invalid")
	}
	normalized := map[string]json.RawMessage{
		"kd_chung": chung, "kd_rieng": rieng,
		"vcnv": mustJSON(map[string]any{"obstacle": jsonValue(obstacle), "obstacle_asset_id": jsonValue(obstacleAsset), "rows": jsonValue(rows), "center": jsonValue(center)}),
		"tt":   ttRaw, "vd": root["vd"],
	}
	return json.Marshal(normalized)
}

func jsonValue(raw json.RawMessage) any {
	var value any
	_ = json.Unmarshal(raw, &value)
	return value
}

func mustJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func validQuestionData(raw json.RawMessage) bool {
	if !json.Valid(raw) {
		return false
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil {
		return false
	}
	for _, key := range []string{"kd_chung", "kd_rieng", "vcnv", "tt", "vd"} {
		if _, exists := keys[key]; !exists {
			return false
		}
	}
	var value struct {
		KDChung []json.RawMessage   `json:"kd_chung"`
		KDRieng [][]json.RawMessage `json:"kd_rieng"`
		VCNV    struct {
			Obstacle        string            `json:"obstacle"`
			ObstacleAssetID string            `json:"obstacle_asset_id"`
			Rows            []json.RawMessage `json:"rows"`
			Center          json.RawMessage   `json:"center"`
		} `json:"vcnv"`
		TT []struct {
			Type string `json:"type"`
		} `json:"tt"`
		VD []json.RawMessage `json:"vd"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	if value.KDChung == nil || len(value.KDRieng) != 4 || len(value.VCNV.Rows) != 4 || strings.TrimSpace(value.VCNV.Obstacle) == "" || value.VD == nil {
		return false
	}
	for _, question := range value.TT {
		if question.Type != "" && question.Type != "text" && question.Type != "image" && question.Type != "video" {
			return false
		}
	}
	var center map[string]any
	if json.Unmarshal(value.VCNV.Center, &center) != nil || center == nil {
		return false
	}
	return true
}

func (s *Service) resolveQuestion(ctx context.Context, id string) (QuestionSnapshot, error) {
	if s.state == nil || s.state.QuestionRevisionID == "" {
		return QuestionSnapshot{}, validation("question", "the selected match has no question revision")
	}
	revision, err := s.repository.GetQuestionSetRevision(ctx, s.state.QuestionRevisionID)
	if err != nil {
		return QuestionSnapshot{}, notFound("question set revision")
	}
	var root any
	if err := json.Unmarshal(revision.Data, &root); err != nil {
		return QuestionSnapshot{}, validation("question", "the pinned question revision is invalid")
	}
	current := root
	for _, segment := range strings.Split(id, ".") {
		switch value := current.(type) {
		case map[string]any:
			current, err = mapSegment(value, segment)
		case []any:
			current, err = sliceSegment(value, segment)
		default:
			err = errors.New("path enters a scalar value")
		}
		if err != nil {
			return QuestionSnapshot{}, validation("question", "question id is not present in the pinned revision")
		}
	}
	raw, _ := json.Marshal(current)
	var question QuestionSnapshot
	if err := json.Unmarshal(raw, &question); err != nil || strings.TrimSpace(question.Content) == "" {
		return QuestionSnapshot{}, validation("question", "question id does not point to a valid question")
	}
	question.ID = id
	if question.Type == "" {
		question.Type = "text"
	}
	return question, nil
}

func mapSegment(value map[string]any, segment string) (any, error) {
	result, ok := value[segment]
	if !ok {
		return nil, errors.New("map key not found")
	}
	return result, nil
}

func sliceSegment(value []any, segment string) (any, error) {
	index, err := strconv.Atoi(segment)
	if err != nil || index < 0 || index >= len(value) {
		return nil, errors.New("array index out of range")
	}
	return value[index], nil
}
func validation(field, message string) error {
	return &opsocket.Error{Code: "VALIDATION", Message: message, Details: map[string]any{"field": field}}
}
func notFound(entity string) error {
	return &opsocket.Error{Code: "NOT_FOUND", Message: entity + " not found"}
}
func conflict(err error) error {
	var operation *opsocket.Error
	if errors.As(err, &operation) {
		return operation
	}
	return &opsocket.Error{Code: "CONFLICT", Message: err.Error(), Retryable: true}
}
