package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

// Policy tests use a tiny durable-state fixture, not bcrypt/database setup per case.
type bellPolicyRepository struct {
	controllerEventRepository
	initial RuntimeState
}

func (r *bellPolicyRepository) GetQuestionSetRevision(context.Context, string) (QuestionSetRevision, error) {
	return QuestionSetRevision{ID: "revision", Data: json.RawMessage(`{"kd_chung":[{"content":"common"},{"content":"next common"}],"kd_rieng":[[{"content":"individual"}],[],[],[]],"vcnv":{"rows":[{"content":"row one"},{"content":"row two"},{},{}],"center":{"content":"center","time":30}},"tt":[{"content":"typed","time":30}],"vd":[{"20":[{"content":"owner one"},{"content":"next owner one"}],"30":[]},{"20":[{"content":"owner two"}],"30":[]},{"20":[],"30":[]},{"20":[],"30":[]}]}`)}, nil
}

func (r *bellPolicyRepository) SelectedMatch(context.Context) (string, int64, error) {
	return r.initial.MatchID, r.initial.Epoch, nil
}

func (r *bellPolicyRepository) LoadRuntime(context.Context, string, int64) (RuntimeState, error) {
	if len(r.snapshots) > 0 {
		return cloneRuntime(r.snapshots[len(r.snapshots)-1]), nil
	}
	return cloneRuntime(r.initial), nil
}

func (r *bellPolicyRepository) SetMatchStatus(_ context.Context, _ string, status MatchStatus, version int64, _ Actor) (Match, error) {
	return Match{Status: status, Version: version + 1}, nil
}

func (r *bellPolicyRepository) CompleteMatch(ctx context.Context, state RuntimeState, event string, payload any, _ []byte, actor Actor, commandID string) (RuntimeState, error) {
	return r.SaveRuntime(ctx, state, event, payload, actor, commandID)
}

func bellPolicyService(t *testing.T, phase controller.Phase, screen controller.Screen, questionID string) (*Service, *bellPolicyRepository) {
	t.Helper()
	ctrl := controller.DefaultState()
	ctrl.Phase, ctrl.Screen = phase, screen
	ctrl.SelectedQuestionID, ctrl.IsShowingQuestion = questionID, questionID != ""
	state := RuntimeState{
		MatchID: "bell-match", Epoch: 2, Version: 10, InputEpoch: 3, BellEpoch: 7,
		Status: StatusPlaying, QuestionRevisionID: "revision", Controller: ctrl,
		Contestants: []ContestantState{{Slot: 1}, {Slot: 2}, {Slot: 3}, {Slot: 4}},
	}
	if questionID != "" {
		state.CurrentQuestion = &QuestionSnapshot{ID: questionID, Content: "question", Answer: "private", Time: 30}
	}
	normalizeBell(&state, time.Now().UnixMilli())
	repository := &bellPolicyRepository{initial: cloneRuntime(state)}
	service := &Service{state: &state, repository: repository}
	t.Cleanup(func() {
		service.mu.Lock()
		defer service.mu.Unlock()
		service.cancelTimerLocked()
	})
	return service, repository
}

func bellCommand(state RuntimeState) Command {
	return Command{MatchID: state.MatchID, ExpectedEpoch: state.Epoch, ExpectedVersion: state.Version,
		ExpectedInputEpoch: state.InputEpoch, ExpectedBellEpoch: state.BellEpoch}
}

func bellMust(t *testing.T, state RuntimeState, err error) RuntimeState {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func bellApply(t *testing.T) func(RuntimeState, error) RuntimeState {
	t.Helper()
	return func(state RuntimeState, err error) RuntimeState {
		t.Helper()
		return bellMust(t, state, err)
	}
}

func TestAutomaticBellPolicyForEveryPhaseAndScreen(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseIdle, controller.PhaseMain, controller.PhaseScore,
		controller.PhaseKhoiDong, controller.PhaseVCNV, controller.PhaseTangToc, controller.PhaseVeDich} {
		for _, screen := range controller.Screens(phase) {
			t.Run(string(phase)+"/"+string(screen), func(t *testing.T) {
				id := "tt.0"
				if phase == controller.PhaseKhoiDong {
					id = "kd_chung.0"
				} else if phase == controller.PhaseVCNV {
					id = "vcnv.rows.0"
				} else if phase == controller.PhaseVeDich {
					id = "vd.0.20.0"
				}
				service, _ := bellPolicyService(t, phase, screen, id)
				service.state.Timer = TimerState{IsRunning: true, DurationMS: stealDurationMS, RemainingMS: stealDurationMS, StartedAt: time.Now().UnixMilli()}
				service.state.Controller.IsAllowingBell = true // forged legacy controller flag
				want := phase == controller.PhaseVCNV || phase == controller.PhaseKhoiDong && (screen == controller.ScreenQuesChung || screen == controller.ScreenTestBell) ||
					phase == controller.PhaseVeDich && screen == controller.ScreenQuesTS1
				for _, principal := range []opsocket.Principal{{}, {Roles: []string{string(auth.RoleAdmin)}}, {Claims: map[string]string{"slot": "2"}}} {
					if got := service.Current(principal).Controller.IsAllowingBell; got != want {
						t.Fatalf("normalized availability=%v, want %v", got, want)
					}
				}
				_, err := service.Ring(context.Background(), 2, bellCommand(*service.state), Actor{})
				if (err == nil) != want {
					t.Fatalf("Ring error=%v, expected allowed=%v", err, want)
				}
			})
		}
	}
}

func TestBellPolicyRejectsHiddenMismatchedAndIndividualQuestions(t *testing.T) {
	for _, test := range []struct {
		name   string
		phase  controller.Phase
		screen controller.Screen
		id     string
		mutate func(*RuntimeState)
	}{
		{name: "KD individual", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesTS1, id: "kd_rieng.0.0"},
		{name: "individual on common screen", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung, id: "kd_rieng.0.0"},
		{name: "hidden common", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung, id: "kd_chung.0", mutate: func(s *RuntimeState) { s.Controller.IsShowingQuestion = false }},
		{name: "no common snapshot", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung, id: "kd_chung.0", mutate: func(s *RuntimeState) { s.CurrentQuestion = nil }},
		{name: "mismatched common snapshot", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung, id: "kd_chung.0", mutate: func(s *RuntimeState) { s.CurrentQuestion.ID = "kd_chung.1" }},
		{name: "wrong phase common", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung, id: "tt.0"},
		{name: "malformed common", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung, id: "kd_chung.invalid"},
		{name: "missing question", phase: controller.PhaseKhoiDong, screen: controller.ScreenQuesChung},
		{name: "hidden steal", phase: controller.PhaseVeDich, screen: controller.ScreenQuesTS1, id: "vd.0.20.0", mutate: func(s *RuntimeState) { s.Controller.IsShowingQuestion = false }},
		{name: "wrong owner screen", phase: controller.PhaseVeDich, screen: controller.ScreenQuesTS2, id: "vd.0.20.0"},
		{name: "mismatched steal snapshot", phase: controller.PhaseVeDich, screen: controller.ScreenQuesTS1, id: "vd.0.20.0", mutate: func(s *RuntimeState) { s.CurrentQuestion.ID = "vd.1.20.0" }},
		{name: "out of range VD slot", phase: controller.PhaseVeDich, screen: controller.ScreenQuesTS1, id: "vd.4.20.0"},
		{name: "wrong phase steal", phase: controller.PhaseVeDich, screen: controller.ScreenQuesTS1, id: "tt.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, repository := bellPolicyService(t, test.phase, test.screen, test.id)
			service.state.Timer = TimerState{IsRunning: true, DurationMS: stealDurationMS, RemainingMS: stealDurationMS, StartedAt: time.Now().UnixMilli()}
			if test.mutate != nil {
				test.mutate(service.state)
			}
			service.state.Controller.IsAllowingBell = true
			if _, err := service.Ring(context.Background(), 2, bellCommand(*service.state), Actor{}); err == nil || len(repository.events) != 0 {
				t.Fatalf("invalid bell accepted or persisted: %v, events=%v", err, repository.events)
			}
			if service.Current(opsocket.Principal{}).Controller.IsAllowingBell {
				t.Fatal("invalid bell advertised as available")
			}
		})
	}
}

func TestCapturedBellRaceHasOneKDOrVDWinnerButFourVCNVRecords(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseKhoiDong, controller.PhaseVCNV, controller.PhaseVeDich} {
		t.Run(string(phase), func(t *testing.T) {
			screen, id := controller.ScreenQuesChung, "kd_chung.0"
			if phase == controller.PhaseVCNV {
				screen, id = controller.ScreenMainVCNV, ""
			} else if phase == controller.PhaseVeDich {
				screen, id = controller.ScreenQuesTS1, "vd.0.20.0"
			}
			service, repository := bellPolicyService(t, phase, screen, id)
			if phase == controller.PhaseVeDich {
				service.state.Timer = TimerState{IsRunning: true, DurationMS: stealDurationMS, RemainingMS: stealDurationMS, StartedAt: time.Now().UnixMilli()}
			}
			captured := bellCommand(*service.state)
			captured.ExpectedInputEpoch = 0 // a bell does not need an answer-window token
			var group sync.WaitGroup
			results := make(chan error, 4)
			for slot := 1; slot <= 4; slot++ {
				group.Add(1)
				go func(slot int) {
					defer group.Done()
					cmd := captured
					cmd.CommandID = fmt.Sprintf("captured-%d", slot)
					_, err := service.Ring(context.Background(), slot, cmd, Actor{})
					results <- err
				}(slot)
			}
			group.Wait()
			close(results)
			accepted := 0
			for err := range results {
				if err == nil {
					accepted++
				}
			}
			want := 1
			if phase == controller.PhaseVCNV {
				want = 4
			}
			if accepted != want || len(repository.events) != want {
				t.Fatalf("accepted=%d events=%d, want %d", accepted, len(repository.events), want)
			}
			if service.state.BellEpoch != captured.ExpectedBellEpoch {
				t.Fatal("winner changed its own captured bell epoch")
			}
			if phase == controller.PhaseVeDich && service.state.Contestants[0].HasRung {
				t.Fatal("question owner won a steal")
			}
			if _, err := service.Ring(context.Background(), 2, bellCommand(*service.state), Actor{}); err == nil {
				t.Fatal("accepted duplicate or competing bell after the claim")
			}
		})
	}
}

func TestStealStartsFiveSecondsAndReplacesOwnerTimer(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
	ctx, must := context.Background(), bellApply(t)
	state := must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state), DurationMS: 5_000}, Actor{}))
	if state.Timer.DurationMS != 15_000 || state.Controller.IsAllowingBell {
		t.Fatal("cached duration override changed the normal owner timer or opened its bell")
	}
	before := state
	state = must(service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Steal: true, DurationMS: 999_999}, Actor{}))
	if state.Timer.DurationMS != stealDurationMS || state.Timer.RemainingMS != stealDurationMS ||
		!state.Timer.IsRunning || state.Timer.IsPaused || !state.Controller.IsAllowingBell || state.BellEpoch == before.BellEpoch {
		t.Fatalf("explicit steal did not replace the owner timer with a fresh five-second window: %+v", state)
	}
	if _, err := service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Steal: true}, Actor{}); err == nil {
		t.Fatal("duplicate steal restarted the five-second deadline")
	}
	captured := bellCommand(state)
	service.state.Controller.SelectedContestant = 4 // untrusted selected slot cannot change owner
	if _, err := service.Ring(ctx, 1, captured, Actor{}); err == nil {
		t.Fatal("question owner stole after selected-slot spoof")
	}
	state = must(service.Ring(ctx, 4, captured, Actor{}))
	if !state.Contestants[3].HasRung || state.Controller.IsAllowingBell {
		t.Fatal("non-owner failed to claim the only steal")
	}
	state = must(service.ResetTimer(ctx, bellCommand(state), Actor{}))
	if _, err := service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Steal: true}, Actor{}); err == nil {
		t.Fatal("reset rearmed an already claimed steal")
	}
}

func TestStealRejectsClosedTimerStatesAndVideos(t *testing.T) {
	for _, scenario := range []string{"normal timer", "paused", "expired before worker", "finished", "reset", "playing video", "paused video", "holding video", "match paused"} {
		t.Run(scenario, func(t *testing.T) {
			service, _ := bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
			service.state.Timer = TimerState{IsRunning: true, DurationMS: stealDurationMS, RemainingMS: stealDurationMS, StartedAt: time.Now().UnixMilli()}
			switch scenario {
			case "normal timer":
				service.state.Timer.DurationMS = 15_000
			case "paused":
				service.state.Timer.IsPaused = true
			case "expired before worker":
				service.state.Timer.StartedAt -= stealDurationMS + 1
			case "finished":
				service.state.Timer.IsRunning, service.state.Timer.RemainingMS = false, 0
			case "reset":
				service.state.Timer = TimerState{}
			case "playing video":
				service.state.Controller.VeDichVideo.Status = controller.VeDichVideoPlaying
			case "paused video":
				service.state.Controller.VeDichVideo.Status = controller.VeDichVideoPaused
			case "holding video":
				service.state.Controller.VeDichVideo.Status = controller.VeDichVideoHolding
			case "match paused":
				service.state.Status = StatusPaused
			}
			service.state.Controller.IsAllowingBell = true
			if service.Current(opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}}).Controller.IsAllowingBell {
				t.Fatal("closed steal advertised as available")
			}
			if _, err := service.Ring(context.Background(), 2, bellCommand(*service.state), Actor{}); err == nil {
				t.Fatal("forged manual flag bypassed the closed steal")
			}
			if scenario == "playing video" || scenario == "paused video" || scenario == "holding video" {
				service.state.Timer = TimerState{}
				if _, err := service.StartTimer(context.Background(), TimerStart{Command: bellCommand(*service.state), Steal: true}, Actor{}); err == nil {
					t.Fatal("steal started during video playback or holding")
				}
			}
		})
	}
}

func TestStealRequiresRevealedMatchingOwnerScreen(t *testing.T) {
	for _, scenario := range []string{"KD common", "VCNV", "TT", "unrevealed", "wrong screen", "wrong snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			service, repository := bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
			switch scenario {
			case "KD common":
				service.state.Controller.Phase, service.state.Controller.Screen = controller.PhaseKhoiDong, controller.ScreenQuesChung
				service.state.Controller.SelectedQuestionID, service.state.CurrentQuestion.ID = "kd_chung.0", "kd_chung.0"
			case "VCNV":
				service.state.Controller.Phase, service.state.Controller.Screen = controller.PhaseVCNV, controller.ScreenQues
				service.state.Controller.SelectedQuestionID, service.state.CurrentQuestion.ID = "vcnv.rows.0", "vcnv.rows.0"
			case "TT":
				service.state.Controller.Phase, service.state.Controller.Screen = controller.PhaseTangToc, controller.ScreenQues
				service.state.Controller.SelectedQuestionID, service.state.CurrentQuestion.ID = "tt.0", "tt.0"
			case "unrevealed":
				service.state.Controller.IsShowingQuestion = false
			case "wrong screen":
				service.state.Controller.Screen = controller.ScreenQuesTS2
			case "wrong snapshot":
				service.state.CurrentQuestion.ID = "vd.1.20.0"
			}
			if _, err := service.StartTimer(context.Background(), TimerStart{Command: bellCommand(*service.state), Steal: true}, Actor{}); err == nil || len(repository.events) != 0 {
				t.Fatalf("invalid explicit steal accepted: %v events=%v", err, repository.events)
			}
		})
	}
}

func TestStealPauseResumeAndResetRejectCapturedOldWindows(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
	ctx, must := context.Background(), bellApply(t)
	state := must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state), Steal: true}, Actor{}))
	old := bellCommand(state)
	state = must(service.PauseTimer(ctx, bellCommand(state), Actor{}))
	if state.Controller.IsAllowingBell {
		t.Fatal("paused steal stayed open")
	}
	if _, err := service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Steal: true}, Actor{}); err == nil {
		t.Fatal("paused duplicate steal was restarted")
	}
	state = must(service.ResumeTimer(ctx, bellCommand(state), Actor{}))
	if !state.Controller.IsAllowingBell || state.BellEpoch == old.ExpectedBellEpoch {
		t.Fatal("resumed steal did not publish a fresh open window")
	}
	if _, err := service.Ring(ctx, 2, old, Actor{}); err == nil {
		t.Fatal("pre-pause press won after resume")
	}
	old = bellCommand(state)
	state = must(service.ResetTimer(ctx, bellCommand(state), Actor{}))
	state = must(service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Steal: true}, Actor{}))
	if _, err := service.Ring(ctx, 2, old, Actor{}); err == nil {
		t.Fatal("previous steal window won after explicit reset/rearm")
	}
	state = must(service.Ring(ctx, 2, bellCommand(state), Actor{}))
	if !state.Contestants[1].HasRung {
		t.Fatal("fresh rearmed steal was rejected")
	}
}

func TestBellClaimsSurviveTimerUnlockAndCachedControllerUpdates(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseKhoiDong, controller.PhaseVeDich} {
		t.Run(string(phase), func(t *testing.T) {
			screen, id, nextID := controller.ScreenQuesChung, "kd_chung.0", "kd_chung.1"
			if phase == controller.PhaseVeDich {
				screen, id, nextID = controller.ScreenQuesTS1, "vd.0.20.0", "vd.0.20.1"
			}
			service, _ := bellPolicyService(t, phase, screen, id)
			ctx, must := context.Background(), bellApply(t)
			if phase == controller.PhaseVeDich {
				must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state), Steal: true}, Actor{}))
			}
			captured := bellCommand(*service.state)
			state := must(service.Ring(ctx, 2, captured, Actor{}))
			state = must(service.SetContestantLock(ctx, 2, false, bellCommand(state), Actor{}))
			state = must(service.ResetTimer(ctx, bellCommand(state), Actor{}))
			state = must(service.StartTimer(ctx, TimerStart{Command: bellCommand(state)}, Actor{}))
			state = must(service.AdjustScore(ctx, 4, 10, "unrelated", bellCommand(state), Actor{}))
			cached := cloneController(state.Controller)
			cached.IsAllowingBell = true
			state = must(service.UpdateController(ctx, cached, bellCommand(state), Actor{}))
			if !state.Contestants[1].HasRung || state.Controller.IsAllowingBell {
				t.Fatal("timer/unlock/cached flags erased or reopened the winning question claim")
			}
			if _, err := service.Ring(ctx, 3, bellCommand(state), Actor{}); err == nil {
				t.Fatal("competing contestant won a claimed question")
			}
			cached.SelectedQuestionID = nextID
			state = must(service.UpdateController(ctx, cached, bellCommand(state), Actor{}))
			if state.Contestants[1].HasRung || state.Controller.IsAllowingBell {
				t.Fatal("genuine new question retained its old claim or was revealed early")
			}
			cached = cloneController(state.Controller)
			cached.IsShowingQuestion = true
			state = must(service.UpdateController(ctx, cached, bellCommand(state), Actor{}))
			if phase == controller.PhaseVeDich {
				state = must(service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Steal: true}, Actor{}))
			}
			if _, err := service.Ring(ctx, 3, captured, Actor{}); err == nil {
				t.Fatal("previous-question captured press won the new question")
			}
			must(service.Ring(ctx, 3, bellCommand(state), Actor{}))
		})
	}
}

func TestVCNVAnswerBellLocksTimingAndPhaseWideFreshness(t *testing.T) {
	service, repository := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenQues, "vcnv.rows.0")
	ctx, must := context.Background(), bellApply(t)
	captured := bellCommand(*service.state)
	state := must(service.StartTimer(ctx, TimerStart{Command: captured}, Actor{}))
	service.state.Timer.StartedAt -= 250
	state = must(service.SubmitAnswer(ctx, 1, "typed answer", bellCommand(state), Actor{}))
	answerTime := state.Contestants[0].TimeMS
	service.state.Timer.StartedAt -= 250
	state = must(service.Ring(ctx, 1, captured, Actor{}))
	if state.Contestants[0].Locked || state.Contestants[0].BellLocked || !state.Contestants[0].HasRung ||
		state.Contestants[0].TimeMS != answerTime || state.Contestants[0].Answer != "typed answer" || !state.Controller.IsAllowingBell {
		t.Fatalf("typed answer incorrectly blocked or was changed by its later bell: %+v", state.Contestants[0])
	}
	var event struct {
		TimeMS int64 `json:"time_ms"`
	}
	if err := json.Unmarshal(repository.payloads[len(repository.payloads)-1], &event); err != nil || event.TimeMS <= answerTime {
		t.Fatalf("bell event did not keep its own later time: %+v, %v", event, err)
	}
	state = must(service.SetContestantLock(ctx, 3, true, bellCommand(state), Actor{}))
	if !state.Contestants[2].BellLocked {
		t.Fatal("explicit operator lock did not lock the bell")
	}
	state = must(service.ResetTimer(ctx, bellCommand(state), Actor{}))
	ctrl := cloneController(state.Controller)
	ctrl.SelectedQuestionID = "vcnv.rows.1"
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	if state.BellEpoch != captured.ExpectedBellEpoch || state.InputEpoch == captured.ExpectedInputEpoch ||
		!state.Contestants[0].HasRung || !state.Contestants[2].BellLocked || !state.Controller.IsAllowingBell {
		t.Fatal("VCNV row change changed the phase bell identity or erased bell records/locks")
	}
	ctrl = cloneController(state.Controller)
	ctrl.IsShowingQuestion = true
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	state = must(service.StartTimer(ctx, TimerStart{Command: bellCommand(state)}, Actor{}))
	state = must(service.PauseTimer(ctx, bellCommand(state), Actor{}))
	state = must(service.ResumeTimer(ctx, bellCommand(state), Actor{}))
	state = must(service.SubmitAnswer(ctx, 3, "typed despite independent bell lock", bellCommand(state), Actor{}))
	if state.Contestants[2].Answer == "" || state.Contestants[2].Locked || !state.Contestants[2].BellLocked {
		t.Fatal("answer submission and explicit bell lock were not independent")
	}
	state = must(service.Ring(ctx, 2, captured, Actor{}))
	if _, err := service.Ring(ctx, 3, captured, Actor{}); err == nil {
		t.Fatal("VCNV manual bell lock vanished across question/timer changes")
	}
	state = must(service.SetContestantLock(ctx, 3, false, bellCommand(state), Actor{}))
	state = must(service.Ring(ctx, 3, captured, Actor{}))
	if _, err := service.Ring(ctx, 1, captured, Actor{}); err == nil {
		t.Fatal("VCNV duplicate ring was accepted after question changes")
	}
	state = must(service.SetContestantLock(ctx, 1, false, bellCommand(state), Actor{}))
	if state.Contestants[0].HasRung || !state.Contestants[1].HasRung ||
		state.BellEpoch != captured.ExpectedBellEpoch {
		t.Fatal("explicit VCNV unlock did not reset only that contestant's bell record")
	}
	state = must(service.Ring(ctx, 1, captured, Actor{}))
	if !state.Contestants[0].HasRung {
		t.Fatal("VCNV contestant could not ring again after explicit operator unlock")
	}
	ctrl = cloneController(state.Controller)
	ctrl.Screen = controller.ScreenMainVCNV
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	state = must(service.Ring(ctx, 4, captured, Actor{}))
	ctrl = cloneController(state.Controller)
	ctrl.Phase, ctrl.Screen = controller.PhaseTangToc, controller.ScreenQues
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	if state.Controller.IsAllowingBell || state.Contestants[0].HasRung || state.Contestants[2].BellLocked {
		t.Fatal("leaving VCNV retained bell eligibility, records, or locks")
	}
	ctrl = cloneController(state.Controller)
	ctrl.Phase, ctrl.Screen = controller.PhaseVCNV, controller.ScreenMainVCNV
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	if _, err := service.Ring(ctx, 1, captured, Actor{}); err == nil {
		t.Fatal("previous-phase bell token was accepted after reentering VCNV")
	}
	must(service.Ring(ctx, 1, bellCommand(state), Actor{}))
}

func TestBellCommandsRequireMatchEpochAndBellEpochButNotAnswerEpoch(t *testing.T) {
	for _, missing := range []string{"match", "epoch", "bell", "wrong match", "wrong epoch", "wrong bell"} {
		t.Run(missing, func(t *testing.T) {
			service, repository := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenMainVCNV, "")
			cmd := bellCommand(*service.state)
			switch missing {
			case "match":
				cmd.MatchID = ""
			case "epoch":
				cmd.ExpectedEpoch = 0
			case "bell":
				cmd.ExpectedBellEpoch = 0
			case "wrong match":
				cmd.MatchID = "other"
			case "wrong epoch":
				cmd.ExpectedEpoch++
			case "wrong bell":
				cmd.ExpectedBellEpoch++
			}
			if _, err := service.Ring(context.Background(), 1, cmd, Actor{}); err == nil || len(repository.events) != 0 {
				t.Fatal("accepted unscoped or stale bell")
			}
		})
	}
	service, _ := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenMainVCNV, "")
	cmd := bellCommand(*service.state)
	cmd.ExpectedVersion, cmd.ExpectedInputEpoch = 999, 999
	bellApply(t)(service.Ring(context.Background(), 1, cmd, Actor{}))
}

func TestCachedManualBellFlagsCannotOverrideAutomaticPolicyOrEmitFalseChanges(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseTangToc, controller.PhaseVCNV, controller.PhaseKhoiDong} {
		t.Run(string(phase), func(t *testing.T) {
			screen, id := controller.ScreenQues, "tt.0"
			if phase == controller.PhaseVCNV {
				screen, id = controller.ScreenMainVCNV, ""
			}
			if phase == controller.PhaseKhoiDong {
				screen, id = controller.ScreenQuesChung, "kd_chung.0"
			}
			service, repository := bellPolicyService(t, phase, screen, id)
			before := cloneRuntime(*service.state)
			cached := cloneController(before.Controller)
			cached.IsAllowingBell = !before.Controller.IsAllowingBell
			got := bellApply(t)(service.UpdateController(context.Background(), cached, bellCommand(before), Actor{}))
			if !reflect.DeepEqual(got, before) || len(repository.events) != 0 {
				t.Fatal("cached manual flag overrode policy or produced a spurious durable update")
			}
		})
	}
}

func TestRecoveryPauseAndCompletionNormalizeBellSnapshots(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseKhoiDong, controller.PhaseVCNV, controller.PhaseTangToc, controller.PhaseVeDich} {
		t.Run(string(phase), func(t *testing.T) {
			screen, id := controller.ScreenQuesChung, "kd_chung.0"
			if phase == controller.PhaseVCNV {
				screen, id = controller.ScreenMainVCNV, ""
			}
			if phase == controller.PhaseTangToc {
				screen, id = controller.ScreenQues, "tt.0"
			}
			if phase == controller.PhaseVeDich {
				screen, id = controller.ScreenQuesTS1, "vd.0.20.0"
			}
			service, repository := bellPolicyService(t, phase, screen, id)
			service.state.Timer = TimerState{IsRunning: true, DurationMS: stealDurationMS, RemainingMS: stealDurationMS, StartedAt: time.Now().UnixMilli()}
			service.state.Controller.IsAllowingBell = true
			repository.initial = cloneRuntime(*service.state)
			recovered, err := NewService(context.Background(), repository, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			state := recovered.Current(opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}})
			if state.Status != StatusPaused || state.Controller.IsAllowingBell || repository.snapshots[0].Controller.IsAllowingBell {
				t.Fatal("recovered paused match published or persisted an open bell")
			}
			if _, err := recovered.Ring(context.Background(), 2, bellCommand(state), Actor{}); err == nil {
				t.Fatal("recovered paused match accepted a bell")
			}
			must := bellApply(t)
			state = must(recovered.Transition(context.Background(), StatusPlaying, bellCommand(state), Actor{}))
			want := phase == controller.PhaseKhoiDong || phase == controller.PhaseVCNV
			if state.Controller.IsAllowingBell != want {
				t.Fatal("match resume bypassed authoritative policy")
			}
			state = must(recovered.Transition(context.Background(), StatusPaused, bellCommand(state), Actor{}))
			state = must(recovered.Transition(context.Background(), StatusCompleted, bellCommand(state), Actor{}))
			if state.Controller.IsAllowingBell || repository.snapshots[len(repository.snapshots)-1].Controller.IsAllowingBell {
				t.Fatal("completed match retained an open bell")
			}
		})
	}
}

func TestTimerWorkerPublishesNormalizedBellWithoutReopeningClaims(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseVeDich, controller.PhaseVCNV} {
		t.Run(string(phase), func(t *testing.T) {
			screen, id := controller.ScreenQuesTS1, "vd.0.20.0"
			if phase == controller.PhaseVCNV {
				screen, id = controller.ScreenQues, "vcnv.rows.0"
			}
			service, repository := bellPolicyService(t, phase, screen, id)
			publications := make(chan RuntimeState, 6)
			service.publish = func(_, _ string, payload any) { publications <- payload.(RuntimeState) }
			service.state.Timer = TimerState{IsRunning: true, DurationMS: stealDurationMS, RemainingMS: stealDurationMS, StartedAt: time.Now().UnixMilli()}
			service.state.Contestants[0].HasRung = true
			service.mu.Lock()
			service.runTimerLocked(1)
			service.mu.Unlock()
			for range 6 {
				select {
				case snapshot := <-publications:
					if snapshot.Controller.IsAllowingBell != (phase == controller.PhaseVCNV) {
						t.Fatal("timer worker published an unsupported or incorrectly closed bell")
					}
				case <-time.After(time.Second):
					t.Fatal("timer worker did not publish its finished snapshots")
				}
			}
			state := service.Current(opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}})
			if state.Timer.IsRunning || !state.Contestants[0].HasRung || len(repository.events) != 1 ||
				repository.events[0] != "Timer.Finished" || repository.snapshots[0].Controller.IsAllowingBell != (phase == controller.PhaseVCNV) {
				t.Fatal("timer finish lost a claim or persisted non-authoritative availability")
			}
		})
	}
}

func TestMatchPauseInvalidatesBellTokensWithoutErasingClaims(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenMainVCNV, "")
	ctx, must := context.Background(), bellApply(t)
	captured := bellCommand(*service.state)
	state := must(service.Ring(ctx, 1, captured, Actor{}))
	state = must(service.Transition(ctx, StatusPaused, bellCommand(state), Actor{}))
	if state.Controller.IsAllowingBell || !state.Contestants[0].HasRung {
		t.Fatal("match pause did not close the bell or erased a phase-wide record")
	}
	state = must(service.Transition(ctx, StatusPlaying, bellCommand(state), Actor{}))
	if _, err := service.Ring(ctx, 2, captured, Actor{}); err == nil {
		t.Fatal("pre-pause captured press won after match resume")
	}
	must(service.Ring(ctx, 2, bellCommand(state), Actor{}))
}

func TestCapturedKDBellsCompeteWhileTimerAndUnlockUpdatesRun(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseKhoiDong, controller.ScreenQuesChung, "kd_chung.0")
	captured := bellCommand(*service.state)
	ctx := context.Background()
	var group sync.WaitGroup
	results := make(chan error, 4)
	for slot := 1; slot <= 4; slot++ {
		group.Add(1)
		go func(slot int) {
			defer group.Done()
			_, err := service.Ring(ctx, slot, captured, Actor{})
			results <- err
		}(slot)
	}
	group.Add(1)
	go func() {
		defer group.Done()
		_, _ = service.StartTimer(ctx, TimerStart{}, Actor{})
		for slot := 1; slot <= 4; slot++ {
			_, _ = service.SetContestantLock(ctx, slot, false, Command{}, Actor{})
		}
		_, _ = service.AdjustScore(ctx, 4, 10, "unrelated", Command{}, Actor{})
	}()
	group.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	state := service.Current(opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}})
	claims := 0
	for _, contestant := range state.Contestants {
		if contestant.HasRung {
			claims++
		}
	}
	if accepted != 1 || claims != 1 || state.Controller.IsAllowingBell {
		t.Fatalf("concurrent timer/unlock updates reopened a KD claim: accepted=%d claims=%d", accepted, claims)
	}
}
