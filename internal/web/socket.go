package web

import (
	"context"
	"log"
	"strconv"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
)

type matchSocket struct {
	service  *game.Service
	sounds   *sound.Service
	settings *settings.Service
}

type selectMatchRequest struct {
	MatchID string `json:"match_id"`
}
type transitionRequest struct{ game.Command }
type scoreRequest struct {
	Command game.Command `json:"command"`
	Slot    int          `json:"slot"`
	Delta   int          `json:"delta"`
	Reason  string       `json:"reason"`
}
type scoresRequest struct {
	Command     game.Command                 `json:"command"`
	Adjustments []game.ScoreAdjustment       `json:"adjustments"`
	Reason      string                       `json:"reason"`
	Sources     []game.ScoreAdjustmentSource `json:"sources,omitempty"`
}
type undoScoresRequest struct {
	Command game.Command `json:"command"`
	UndoOf  string       `json:"undo_of"`
}
type contestantLockRequest struct {
	Command game.Command `json:"command"`
	Slot    int          `json:"slot"`
	Locked  bool         `json:"locked"`
}
type amendResultRequest struct {
	MatchID string `json:"match_id"`
	Result  []byte `json:"result"`
	Reason  string `json:"reason"`
}

func (m *matchSocket) GetCurrent(ctx *opsocket.Ctx, _ struct{}) (game.RuntimeState, error) {
	return m.service.Current(ctx.Principal()), nil
}
func (m *matchSocket) Select(ctx *opsocket.Ctx, request selectMatchRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.SelectMatch(ctx.Context(), request.MatchID, actor(ctx))
}
func (m *matchSocket) Unload(ctx *opsocket.Ctx, request transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.UnloadMatch(ctx.Context(), request.Command, actor(ctx))
}
func (m *matchSocket) Start(ctx *opsocket.Ctx, request transitionRequest) (game.RuntimeState, error) {
	return m.transition(ctx, game.StatusPlaying, request.Command)
}
func (m *matchSocket) Pause(ctx *opsocket.Ctx, request transitionRequest) (game.RuntimeState, error) {
	return m.transition(ctx, game.StatusPaused, request.Command)
}
func (m *matchSocket) Resume(ctx *opsocket.Ctx, request transitionRequest) (game.RuntimeState, error) {
	return m.transition(ctx, game.StatusPlaying, request.Command)
}
func (m *matchSocket) Complete(ctx *opsocket.Ctx, request transitionRequest) (game.RuntimeState, error) {
	return m.transition(ctx, game.StatusCompleted, request.Command)
}
func (m *matchSocket) Ready(ctx *opsocket.Ctx, request transitionRequest) (game.RuntimeState, error) {
	return m.transition(ctx, game.StatusReady, request.Command)
}
func (m *matchSocket) AdjustScore(ctx *opsocket.Ctx, request scoreRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.AdjustScore(ctx.Context(), request.Slot, request.Delta, request.Reason, request.Command, actor(ctx))
}
func (m *matchSocket) AdjustScores(ctx *opsocket.Ctx, request scoresRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.AdjustScores(ctx.Context(), request.Adjustments, request.Reason, request.Command, actor(ctx), request.Sources...)
}
func (m *matchSocket) UndoScores(ctx *opsocket.Ctx, request undoScoresRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.UndoScores(ctx.Context(), request.UndoOf, request.Command, actor(ctx))
}
func (m *matchSocket) JudgeKhoiDong(ctx *opsocket.Ctx, request game.KhoiDongJudgmentRequest) (game.KhoiDongJudgmentResult, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.KhoiDongJudgmentResult{}, err
	}
	scoring := settings.DefaultScoring().KhoiDong
	if m.settings != nil {
		scoring = m.settings.Get().Scoring.KhoiDong
	}
	result, err := m.service.JudgeKhoiDong(ctx.Context(), request, scoring, actor(ctx))
	if err != nil {
		return game.KhoiDongJudgmentResult{}, err
	}
	return result, nil
}
func (m *matchSocket) SetContestantLock(ctx *opsocket.Ctx, request contestantLockRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.SetContestantLock(ctx.Context(), request.Slot, request.Locked, request.Command, actor(ctx))
}
func (m *matchSocket) transition(ctx *opsocket.Ctx, status game.MatchStatus, command game.Command) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return m.service.Transition(ctx.Context(), status, command, actor(ctx))
}

type controllerSocket struct {
	service *game.Service
	sounds  *sound.Service
}
type controllerUpdateRequest struct {
	Command game.Command     `json:"command"`
	State   controller.State `json:"state"`
}
type controllerStringRequest struct {
	Command game.Command `json:"command"`
	Value   string       `json:"value"`
}
type controllerIntRequest struct {
	Command game.Command `json:"command"`
	Value   int          `json:"value"`
}
type controllerBoolRequest struct {
	Command game.Command `json:"command"`
	Value   bool         `json:"value"`
}
type controllerVCNVRowRequest struct {
	Command game.Command `json:"command"`
	Row     int          `json:"row"`
	State   string       `json:"state"`
}
type controllerVCNVPanelRequest struct {
	Command game.Command `json:"command"`
	Index   int          `json:"index"`
	Open    bool         `json:"open"`
}
type controllerVDQuestionsRequest struct {
	Command     game.Command `json:"command"`
	Slot        int          `json:"slot"`
	QuestionIDs []string     `json:"question_ids"`
}
type controllerVDStarRequest struct {
	Command  game.Command `json:"command"`
	Slot     int          `json:"slot"`
	Question int          `json:"question"`
}
type controllerVeDichVideoRequest struct {
	Command    game.Command `json:"command"`
	Action     string       `json:"action"`
	QuestionID string       `json:"question_id,omitempty"`
	SessionID  uint64       `json:"session_id,omitempty"`
	Volume     float64      `json:"volume,omitempty"`
	DurationMS int64        `json:"duration_ms,omitempty"`
}

type controllerAnswerStatusRequest struct {
	Command game.Command `json:"command"`
	Slot    int          `json:"slot"`
	Correct bool         `json:"correct"`
}

func (c *controllerSocket) Get(ctx *opsocket.Ctx, _ struct{}) (controller.State, error) {
	return c.service.Controller(ctx.Context()), nil
}
func (c *controllerSocket) Update(ctx *opsocket.Ctx, request controllerUpdateRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.UpdateController(ctx.Context(), request.State, request.Command, actor(ctx))
}
func (c *controllerSocket) ChangePhase(ctx *opsocket.Ctx, r controllerStringRequest) (game.RuntimeState, error) {
	state := c.service.Controller(ctx.Context())
	changed := state.Phase != controller.Phase(r.Value)
	state.Phase = controller.Phase(r.Value)
	if changed {
		state.Screen = controller.DefaultScreen(state.Phase)
	}
	// Screen sounds are cues from the server sound director; effects are never
	// cut by a phase or screen change.
	return c.update(ctx, r.Command, state)
}
func (c *controllerSocket) ChangeScreen(ctx *opsocket.Ctx, r controllerStringRequest) (game.RuntimeState, error) {
	state := c.service.Controller(ctx.Context())
	state.Screen = controller.Screen(r.Value)
	return c.update(ctx, r.Command, state)
}
func (c *controllerSocket) SelectQuestion(ctx *opsocket.Ctx, r controllerStringRequest) (game.RuntimeState, error) {
	state := c.service.Controller(ctx.Context())
	state.SelectedQuestionID = r.Value
	return c.update(ctx, r.Command, state)
}
func (c *controllerSocket) SelectContestant(ctx *opsocket.Ctx, r controllerIntRequest) (game.RuntimeState, error) {
	state := c.service.Controller(ctx.Context())
	state.SelectedContestant = r.Value
	return c.update(ctx, r.Command, state)
}
func (c *controllerSocket) ToggleBell(ctx *opsocket.Ctx, r controllerBoolRequest) (game.RuntimeState, error) {
	state := c.service.Controller(ctx.Context())
	state.IsAllowingBell = r.Value
	return c.update(ctx, r.Command, state)
}
func (c *controllerSocket) ToggleQuestion(ctx *opsocket.Ctx, r controllerBoolRequest) (game.RuntimeState, error) {
	state := c.service.Controller(ctx.Context())
	state.IsShowingQuestion = r.Value
	return c.update(ctx, r.Command, state)
}
func (c *controllerSocket) SetVCNVStarted(ctx *opsocket.Ctx, r controllerBoolRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SetVCNVStarted(ctx.Context(), r.Value, r.Command, actor(ctx))
}
func (c *controllerSocket) SetVCNVRowState(ctx *opsocket.Ctx, r controllerVCNVRowRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SetVCNVRowState(ctx.Context(), r.Row, r.State, r.Command, actor(ctx))
}
func (c *controllerSocket) SetVCNVPanelOpen(ctx *opsocket.Ctx, r controllerVCNVPanelRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SetVCNVPanelOpen(ctx.Context(), r.Index, r.Open, r.Command, actor(ctx))
}
func (c *controllerSocket) SetVCNVSolved(ctx *opsocket.Ctx, r controllerBoolRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SetVCNVSolved(ctx.Context(), r.Value, r.Command, actor(ctx))
}
func (c *controllerSocket) SelectVeDichQuestionSet(ctx *opsocket.Ctx, r controllerVDQuestionsRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SelectVeDichQuestionSet(ctx.Context(), r.Slot, r.QuestionIDs, r.Command, actor(ctx))
}
func (c *controllerSocket) SetVeDichStar(ctx *opsocket.Ctx, r controllerVDStarRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SetVeDichStar(ctx.Context(), r.Slot, r.Question, r.Command, actor(ctx))
}
func (c *controllerSocket) VeDichVideo(ctx *opsocket.Ctx, request controllerVeDichVideoRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.VeDichVideo(ctx.Context(), game.VeDichVideoCommand{
		Command: request.Command, Action: request.Action, QuestionID: request.QuestionID,
		SessionID: request.SessionID, Volume: request.Volume, DurationMS: request.DurationMS,
	}, actor(ctx))
}
func (c *controllerSocket) SetAnswerStatus(ctx *opsocket.Ctx, r controllerAnswerStatusRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.SetAnswerStatus(ctx.Context(), r.Slot, r.Correct, r.Command, actor(ctx))
}

// ClearBells lets every contestant ring again (bell test and any phase).
func (c *controllerSocket) ClearBells(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.ClearBells(ctx.Context(), r.Command, actor(ctx))
}

// ClearAnswers clears every contestant's typed answer and answer time.
func (c *controllerSocket) ClearAnswers(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.ClearAnswers(ctx.Context(), r.Command, actor(ctx))
}
func (c *controllerSocket) Reset(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	updated, err := c.service.ResetController(ctx.Context(), r.Command, actor(ctx))
	if err == nil {
		c.sounds.StopAll(0)
	}
	return updated, err
}
func (c *controllerSocket) update(ctx *opsocket.Ctx, command game.Command, state controller.State) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return c.service.UpdateController(ctx.Context(), state, command, actor(ctx))
}

type timerSocket struct{ service *game.Service }

func (t *timerSocket) Get(ctx *opsocket.Ctx, _ struct{}) (game.RuntimeState, error) {
	return t.service.Current(ctx.Principal()), nil
}
func (t *timerSocket) Start(ctx *opsocket.Ctx, r game.TimerStart) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return t.service.StartTimer(ctx.Context(), r, actor(ctx))
}
func (t *timerSocket) Pause(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return t.service.PauseTimer(ctx.Context(), r.Command, actor(ctx))
}
func (t *timerSocket) Resume(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return t.service.ResumeTimer(ctx.Context(), r.Command, actor(ctx))
}
func (t *timerSocket) Reset(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return game.RuntimeState{}, err
	}
	return t.service.ResetTimer(ctx.Context(), r.Command, actor(ctx))
}

type contestantSocket struct {
	service *game.Service
	sounds  *sound.Service
}
type answerRequest struct {
	Command game.Command `json:"command"`
	Answer  string       `json:"answer"`
}

func (c *contestantSocket) SubmitAnswer(ctx *opsocket.Ctx, r answerRequest) (game.RuntimeState, error) {
	slot, err := contestantSlot(ctx)
	if err != nil {
		return game.RuntimeState{}, err
	}
	updated, err := c.service.SubmitAnswer(ctx.Context(), slot, r.Answer, r.Command, actor(ctx))
	if err != nil {
		return game.RuntimeState{}, err
	}
	return game.FilterRuntime(updated, ctx.Principal()), nil
}
func (c *contestantSocket) Ring(ctx *opsocket.Ctx, r transitionRequest) (game.RuntimeState, error) {
	slot, err := contestantSlot(ctx)
	if err != nil {
		return game.RuntimeState{}, err
	}
	// The bell sound is a server cue (sound director), not played here.
	updated, err := c.service.Ring(ctx.Context(), slot, r.Command, actor(ctx))
	if err != nil {
		return game.RuntimeState{}, err
	}
	return game.FilterRuntime(updated, ctx.Principal()), nil
}
func contestantSlot(ctx *opsocket.Ctx) (int, error) {
	if err := ctx.RequireRole(string(auth.RoleContestant)); err != nil {
		return 0, err
	}
	slot, _ := strconv.Atoi(ctx.Principal().Claims["slot"])
	if slot < 1 || slot > 4 {
		return 0, &opsocket.Error{Code: "FORBIDDEN", Message: "contestant slot is not assigned"}
	}
	return slot, nil
}

type soundSocket struct {
	service *sound.Service
	games   *game.Service
}

func (s *soundSocket) EnableOutput(ctx *opsocket.Ctx, _ struct{}) (sound.Status, error) {
	ctx.Join(sound.OutputRoom)
	return s.service.Status(), nil
}

func (s *soundSocket) DisableOutput(ctx *opsocket.Ctx, _ struct{}) (sound.Status, error) {
	ctx.Leave(sound.OutputRoom)
	return s.service.Status(), nil
}

func (s *soundSocket) GetStatus(_ *opsocket.Ctx, _ struct{}) (sound.Status, error) {
	return s.service.Status(), nil
}

func (s *soundSocket) Play(ctx *opsocket.Ctx, request sound.PlayRequest) (sound.Playback, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return sound.Playback{}, err
	}
	playback, err := s.service.Play(ctx.Context(), request.SoundID)
	if err == nil {
		_ = s.games.RecordAudit(ctx.Context(), game.AuditEntry{Action: "sound.play", EntityType: "sound", EntityID: request.SoundID}, actor(ctx))
	}
	return playback, err
}

func (s *soundSocket) Stop(ctx *opsocket.Ctx, request sound.StopRequest) error {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return err
	}
	s.service.Stop(request)
	_ = s.games.RecordAudit(ctx.Context(), game.AuditEntry{Action: "sound.stop", EntityType: "sound", EntityID: request.PlaybackID}, actor(ctx))
	return nil
}

func (s *soundSocket) StopBus(ctx *opsocket.Ctx, request sound.StopBusRequest) error {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return err
	}
	return s.service.StopBus(request)
}

func (s *soundSocket) StopAll(ctx *opsocket.Ctx, request sound.StopRequest) error {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return err
	}
	s.service.StopAll(request.FadeMS)
	_ = s.games.RecordAudit(ctx.Context(), game.AuditEntry{Action: "sound.stop_all", EntityType: "sound", EntityID: "all"}, actor(ctx))
	return nil
}

// PlayQuestionMedia plays the current question's audio through the TV sound output.
func (s *soundSocket) PlayQuestionMedia(ctx *opsocket.Ctx, _ struct{}) (sound.Playback, error) {
	if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
		return sound.Playback{}, err
	}
	state := s.games.Current(ctx.Principal())
	playback, err := s.service.PlayQuestionMedia(state.CurrentQuestion)
	if err == nil {
		_ = s.games.RecordAudit(ctx.Context(), game.AuditEntry{Action: "sound.play_question_media", EntityType: "question", EntityID: state.Controller.SelectedQuestionID}, actor(ctx))
	}
	return playback, err
}

func (s *soundSocket) TestOutput(ctx *opsocket.Ctx, request sound.TestRequest) (sound.Playback, error) {
	playback, err := s.service.Test(ctx.Context())
	if err != nil {
		return sound.Playback{}, err
	}
	switch request.Scope {
	case "this":
		if err := ctx.EmitToThisConn("Sound.Played", playback); err != nil {
			return sound.Playback{}, err
		}
	case "all":
		if err := ctx.RequireRole(string(auth.RoleAdmin)); err != nil {
			return sound.Playback{}, err
		}
		s.service.Broadcast(playback)
		_ = s.games.RecordAudit(ctx.Context(), game.AuditEntry{Action: "sound.test_all", EntityType: "sound", EntityID: playback.SoundID}, actor(ctx))
	default:
		return sound.Playback{}, &opsocket.Error{Code: "BAD_REQUEST", Message: "test scope must be this or all"}
	}
	return playback, nil
}

func RegisterSocketControllers(socket *opsocket.Socket, games *game.Service, sounds *sound.Service, siteSettings ...*settings.Service) error {
	var site *settings.Service
	if len(siteSettings) > 0 {
		site = siteSettings[0]
	}
	if games != nil && sounds != nil {
		games.SetVerdictSound(func(verdict string, who game.Actor) error {
			id := "builtin.kd.wronganswer"
			if verdict == "correct" {
				id = "builtin.kd.correctanswer"
			}
			// Runs inside the game lock, after commit and before broadcast: keep it to the
			// sound service and the audit append, which never re-enter the game service.
			playCtx := context.Background()
			if _, err := sounds.Play(playCtx, id); err != nil {
				log.Printf("KhoiDong verdict sound %s: %v", id, err)
				return err
			}
			if err := games.RecordAudit(playCtx, game.AuditEntry{Action: "sound.play", EntityType: "sound", EntityID: id}, who); err != nil {
				log.Printf("KhoiDong verdict audit %s: %v", id, err)
			}
			return nil
		})
	}
	for namespace, ctrl := range map[string]any{"Match": &matchSocket{service: games, sounds: sounds, settings: site}, "Controller": &controllerSocket{games, sounds}, "Timer": &timerSocket{games}, "Contestant": &contestantSocket{games, sounds}, "Sound": &soundSocket{sounds, games}} {
		if err := socket.RegisterRequestController(namespace, ctrl); err != nil {
			return err
		}
	}
	return nil
}

func registerSocketControllers(socket *opsocket.Socket, games *game.Service, sounds *sound.Service, siteSettings ...*settings.Service) error {
	return RegisterSocketControllers(socket, games, sounds, siteSettings...)
}

func actor(ctx *opsocket.Ctx) game.Actor {
	userID, _ := strconv.ParseInt(ctx.Principal().Subject, 10, 64)
	return game.Actor{UserID: userID, ClientID: ctx.ClientID()}
}
