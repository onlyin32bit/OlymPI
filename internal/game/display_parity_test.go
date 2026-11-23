package game

import (
	"context"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

var parityAdminPrincipal = opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}}

func stopVideoSchedule(t *testing.T, service *Service) {
	t.Cleanup(func() {
		service.mu.Lock()
		defer service.mu.Unlock()
		service.cancelVideoLocked()
	})
}

func TestTestBellAcceptsOneRingUntilOperatorClears(t *testing.T) {
	service, repository := bellPolicyService(t, controller.PhaseKhoiDong, controller.ScreenTestBell, "")
	ctx, must := context.Background(), bellApply(t)
	state := service.Current(parityAdminPrincipal)
	if !state.Controller.IsAllowingBell {
		t.Fatal("test bell screen did not open the bell")
	}
	state = must(service.Ring(ctx, 2, bellCommand(state), Actor{}))
	if state.Controller.IsAllowingBell {
		t.Fatal("bell stayed open after the first test ring")
	}
	if _, err := service.Ring(ctx, 3, bellCommand(state), Actor{}); err == nil {
		t.Fatal("a second contestant rang before the operator cleared")
	}
	epoch := state.BellEpoch
	state = must(service.ClearBells(ctx, bellCommand(state), Actor{}))
	if state.BellEpoch != epoch+1 || !state.Controller.IsAllowingBell || repository.events[len(repository.events)-1] != "Contestant.BellsCleared" {
		t.Fatalf("clear did not reopen the bell: epoch=%d allowing=%v events=%v", state.BellEpoch, state.Controller.IsAllowingBell, repository.events)
	}
	for _, contestant := range state.Contestants {
		if contestant.HasRung || contestant.BellLocked || contestant.TimeMS != 0 {
			t.Fatalf("clear left a bell claim: %+v", contestant)
		}
	}
	state = must(service.Ring(ctx, 3, bellCommand(state), Actor{}))
	if !state.Contestants[2].HasRung {
		t.Fatal("ring after clear was not recorded")
	}

	// Leaving the bell check never carries a test ring into the real round.
	ctrl := state.Controller
	ctrl.Screen = controller.ScreenQuesChung
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	for _, contestant := range state.Contestants {
		if contestant.HasRung {
			t.Fatalf("test ring survived the screen change: %+v", contestant)
		}
	}
}

func TestClearBellsWorksInAnyPhaseAndKeepsAnswerTimes(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenMainVCNV, "")
	ctx, must := context.Background(), bellApply(t)
	service.state.Contestants[0] = ContestantState{Slot: 1, Answer: "TYPED", TimeMS: 900, HasRung: true, BellLocked: true}
	service.state.Contestants[1] = ContestantState{Slot: 2, TimeMS: 400, HasRung: true}
	state := must(service.ClearBells(ctx, bellCommand(*service.state), Actor{}))
	if got := state.Contestants[0]; got.HasRung || got.BellLocked || got.TimeMS != 900 || got.Answer != "TYPED" {
		t.Fatalf("typed answer time was not preserved: %+v", got)
	}
	if got := state.Contestants[1]; got.HasRung || got.TimeMS != 0 {
		t.Fatalf("bell time was not cleared: %+v", got)
	}
}

func TestClearAnswersKeepsBellsAndLocksAndAdvancesInputEpoch(t *testing.T) {
	service, repository := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenAnswersVCNV, "vcnv.rows.0")
	ctx, must := context.Background(), bellApply(t)
	service.state.Contestants[0] = ContestantState{Slot: 1, Answer: "TYPED", TimeMS: 900, HasRung: true, BellLocked: true, Locked: true}
	service.state.Contestants[1] = ContestantState{Slot: 2, TimeMS: 400, HasRung: true}
	service.state.Controller.AnswerStatus = []bool{false, true, false, true}
	inputEpoch := service.state.InputEpoch
	bellEpoch := service.state.BellEpoch
	state := must(service.ClearAnswers(ctx, bellCommand(*service.state), Actor{}))
	if state.InputEpoch != inputEpoch+1 || state.BellEpoch != bellEpoch || repository.events[len(repository.events)-1] != "Contestant.AnswersCleared" {
		t.Fatalf("unexpected clear answers state: input=%d bell=%d events=%v", state.InputEpoch, state.BellEpoch, repository.events)
	}
	if got := state.Contestants[0]; got.Answer != "" || got.TimeMS != 0 || !got.HasRung || !got.BellLocked || !got.Locked {
		t.Fatalf("clear answers touched bell/lock state: %+v", got)
	}
	if got := state.Contestants[1]; got.Answer != "" || got.TimeMS != 400 || !got.HasRung {
		t.Fatalf("clear answers erased bell-only time: %+v", got)
	}
	for i, ok := range state.Controller.AnswerStatus {
		if !ok {
			t.Fatalf("answer status[%d] was not reset: %+v", i, state.Controller.AnswerStatus)
		}
	}
}

func TestObstacleTimerRunsOnBoardWithoutResettingInputs(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseVCNV, controller.ScreenMainVCNV, "")
	ctx, must := context.Background(), bellApply(t)
	service.state.Contestants[0] = ContestantState{Slot: 1, Answer: "KEPT", TimeMS: 700, HasRung: true}
	inputEpoch := service.state.InputEpoch
	state := must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state), Obstacle: true}, Actor{}))
	if state.Timer.Kind != TimerKindObstacle || state.Timer.DurationMS != 15_000 || !state.Timer.IsRunning {
		t.Fatalf("obstacle timer = %+v", state.Timer)
	}
	if state.InputEpoch != inputEpoch || state.Contestants[0].Answer != "KEPT" || !state.Contestants[0].HasRung {
		t.Fatalf("obstacle timer reset inputs: epoch %d→%d %+v", inputEpoch, state.InputEpoch, state.Contestants[0])
	}
	if _, err := service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Obstacle: true}, Actor{}); err == nil {
		t.Fatal("a second obstacle timer started over a running one")
	}
	ctrl := state.Controller
	ctrl.Screen = controller.ScreenImageVCNV
	state = must(service.UpdateController(ctx, ctrl, bellCommand(state), Actor{}))
	if state.Timer.IsRunning || state.Timer.RemainingMS <= 0 || state.Timer.Kind != TimerKindObstacle {
		t.Fatalf("screen change did not freeze the obstacle timer: %+v", state.Timer)
	}
	state = must(service.StartTimer(ctx, TimerStart{Command: bellCommand(state), Obstacle: true}, Actor{}))
	if !state.Timer.IsRunning || state.Timer.RemainingMS != 15_000 {
		t.Fatalf("obstacle timer did not restart on the image screen: %+v", state.Timer)
	}

	for _, screen := range []controller.Screen{controller.ScreenQues, controller.ScreenStart} {
		other, _ := bellPolicyService(t, controller.PhaseVCNV, screen, "vcnv.rows.0")
		if _, err := other.StartTimer(ctx, TimerStart{Command: bellCommand(*other.state), Obstacle: true}, Actor{}); err == nil {
			t.Fatalf("obstacle timer started on %s", screen)
		}
	}
	other, _ := bellPolicyService(t, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	if _, err := other.StartTimer(ctx, TimerStart{Command: bellCommand(*other.state), Obstacle: true}, Actor{}); err == nil {
		t.Fatal("obstacle timer started outside VCNV")
	}
}

func TestTimerKindIsSetForQuestionAndSteal(t *testing.T) {
	ctx, must := context.Background(), bellApply(t)
	service, _ := bellPolicyService(t, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	if state := must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state)}, Actor{})); state.Timer.Kind != TimerKindQuestion {
		t.Fatalf("question timer kind = %q", state.Timer.Kind)
	}
	service, _ = bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
	if state := must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state), Steal: true}, Actor{})); state.Timer.Kind != TimerKindSteal {
		t.Fatalf("steal timer kind = %q", state.Timer.Kind)
	}
}

func TestVCNVCentreDefaultsToFifteenSeconds(t *testing.T) {
	state := controller.DefaultState()
	state.Phase, state.SelectedQuestionID = controller.PhaseVCNV, "vcnv.center"
	if got, err := questionTimerDuration(state, &QuestionSnapshot{ID: "vcnv.center"}); err != nil || got != 15_000 {
		t.Fatalf("centre default = %d, %v", got, err)
	}
	if got, err := questionTimerDuration(state, &QuestionSnapshot{ID: "vcnv.center", Time: 25}); err != nil || got != 25_000 {
		t.Fatalf("centre question-set time = %d, %v", got, err)
	}
}

func TestHiddenQuestionIsAStubAndSolveRevealsExplanation(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	service.state.CurrentQuestion = &QuestionSnapshot{ID: "tt.0", Content: "secret", Answer: "ans", Type: "image", AssetID: "img", ExplanationAssetID: "exp", Time: 20, Keyword: "kw", CharCount: 4}
	service.state.Controller.IsShowingQuestion = false
	contestant := opsocket.Principal{Roles: []string{string(auth.RoleContestant)}, Claims: map[string]string{"slot": "1"}}
	want := QuestionSnapshot{ID: "tt.0", Type: "image", AssetID: "img", Time: 20, CharCount: 4}
	for _, principal := range []opsocket.Principal{{}, contestant} {
		got := service.Current(principal).CurrentQuestion
		if got == nil || *got != want {
			t.Fatalf("hidden question = %+v, want stub %+v", got, want)
		}
	}
	if got := service.Current(parityAdminPrincipal).CurrentQuestion; got.Content != "secret" || got.Answer != "ans" {
		t.Fatalf("admin lost the full question: %+v", got)
	}

	service.state.Controller.IsShowingQuestion = true
	if got := service.Current(opsocket.Principal{}).CurrentQuestion; got.Content != "secret" || got.Answer != "" || got.Keyword != "" || got.ExplanationAssetID != "" {
		t.Fatalf("shown question leaked answer or explanation: %+v", got)
	}

	service.state.Controller.Screen = controller.ScreenSolve
	service.state.Controller.IsShowingQuestion = false
	if got := service.Current(opsocket.Principal{}).CurrentQuestion; got.Content != "secret" || got.ExplanationAssetID != "exp" || got.Answer != "ans" {
		t.Fatalf("TT solve did not publish text, explanation and answer: %+v", got)
	}
}

func TestKDEndCardShowsWithoutASelectedQuestion(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseKhoiDong, controller.ScreenQuesTS1, "")
	ctx, must := context.Background(), bellApply(t)
	ctrl := service.state.Controller
	ctrl.IsShowingQuestion = true
	state := must(service.UpdateController(ctx, ctrl, bellCommand(*service.state), Actor{}))
	if !state.Controller.IsShowingQuestion || state.Controller.SelectedQuestionID != "" || state.CurrentQuestion != nil {
		t.Fatalf("KẾT THÚC card state = %+v", state.Controller)
	}
}

func TestShutdownAndRecoveryKeepRemainingTime(t *testing.T) {
	service, repository := bellPolicyService(t, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	ctx, must := context.Background(), bellApply(t)
	state := must(service.StartTimer(ctx, TimerStart{Command: bellCommand(*service.state)}, Actor{}))
	time.Sleep(30 * time.Millisecond)
	if err := service.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	saved := repository.snapshots[len(repository.snapshots)-1]
	if !saved.Timer.IsPaused || saved.Timer.RemainingMS >= state.Timer.DurationMS || saved.Timer.RemainingMS < state.Timer.DurationMS-1_000 {
		t.Fatalf("shutdown did not pause with the remaining time: %+v", saved.Timer)
	}
	time.Sleep(30 * time.Millisecond) // downtime
	recovered, err := NewService(ctx, repository, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := recovered.Current(parityAdminPrincipal)
	if got.Status != StatusPaused || !got.Timer.IsPaused || got.Timer.RemainingMS != saved.Timer.RemainingMS || got.Timer.Kind != TimerKindQuestion {
		t.Fatalf("recovered timer = %+v (status %s), want remaining %d", got.Timer, got.Status, saved.Timer.RemainingMS)
	}
	got = must(recovered.Transition(ctx, StatusPlaying, bellCommand(got), Actor{}))
	got = must(recovered.ResumeTimer(ctx, bellCommand(got), Actor{}))
	if !got.Timer.IsRunning || got.Timer.RemainingMS != saved.Timer.RemainingMS {
		t.Fatalf("resumed timer = %+v", got.Timer)
	}
	recovered.mu.Lock()
	recovered.cancelTimerLocked()
	recovered.mu.Unlock()
}

func TestCrashRecoveryFreezesTimerAtLastWrite(t *testing.T) {
	service, repository := bellPolicyService(t, controller.PhaseTangToc, controller.ScreenQues, "tt.0")
	_ = service
	startedAt := time.Now().Add(-time.Minute).UnixMilli()
	repository.initial.Timer = TimerState{IsRunning: true, DurationMS: 30_000, RemainingMS: 30_000, StartedAt: startedAt, Kind: TimerKindQuestion}
	repository.initial.UpdatedAt = time.UnixMilli(startedAt + 4_000)
	recovered, err := NewService(context.Background(), repository, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := recovered.Current(parityAdminPrincipal).Timer
	if !got.IsPaused || !got.IsRunning || got.RemainingMS != 26_000 || got.ElapsedMS != 4_000 {
		t.Fatalf("crash-recovered timer = %+v, want 26 s left", got)
	}
}

func TestVeDichVideoServerClockFinishesAfterHold(t *testing.T) {
	service, repository := bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
	stopVideoSchedule(t, service)
	ctx, must := context.Background(), bellApply(t)
	service.state.Controller.IsShowingQuestion = false
	service.state.CurrentQuestion.Type, service.state.CurrentQuestion.AssetID = "video", "clip"
	state := must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(*service.state), Action: "start"}, Actor{}))
	video := state.Controller.VeDichVideo
	if video.Status != controller.VeDichVideoPlaying || video.StartedAt == 0 || video.PositionMS != 0 || video.DurationMS != 0 {
		t.Fatalf("started video clock = %+v", video)
	}
	time.Sleep(20 * time.Millisecond)
	state = must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(state), Action: "pause"}, Actor{}))
	video = state.Controller.VeDichVideo
	if video.StartedAt != 0 || video.PositionMS < 20 {
		t.Fatalf("pause did not keep the position: %+v", video)
	}
	paused := video.PositionMS
	if _, err := service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(state), Action: "duration", SessionID: video.SessionID + 1, DurationMS: 100}, Actor{}); err == nil {
		t.Fatal("duration for another session was accepted")
	}
	state = must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(state), Action: "duration", SessionID: video.SessionID, DurationMS: paused + 100}, Actor{}))
	events := len(repository.events)
	state = must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(state), Action: "duration", SessionID: video.SessionID, DurationMS: paused + 100}, Actor{}))
	if len(repository.events) != events {
		t.Fatal("repeated duration report persisted an event")
	}
	state = must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(state), Action: "resume"}, Actor{}))
	video = state.Controller.VeDichVideo
	if video.StartedAt == 0 || video.PositionMS != paused || video.DurationMS != paused+100 {
		t.Fatalf("resume clock = %+v", video)
	}
	// Remaining media (100 ms) + the 2 s final-frame hold.
	deadline := time.Now().Add(4 * time.Second)
	for {
		current := service.Current(parityAdminPrincipal)
		if current.Controller.VeDichVideo.Status == controller.VeDichVideoIdle {
			if !current.Timer.IsRunning || current.Timer.Kind != TimerKindQuestion || current.Timer.DurationMS != 15_000 {
				t.Fatalf("server finish did not start the question timer: %+v", current.Timer)
			}
			if elapsed := time.Now().UnixMilli() - video.StartedAt; elapsed < 2_000 {
				t.Fatalf("server finished before the final-frame hold: %d ms", elapsed)
			}
			replay := must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(current), Action: "finish", SessionID: video.SessionID, QuestionID: video.QuestionID}, Actor{}))
			if replay.Version != current.Version {
				t.Fatal("late client finish changed the runtime")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never finished the video: %+v", current.Controller.VeDichVideo)
		}
		time.Sleep(20 * time.Millisecond)
	}
	service.mu.Lock()
	service.cancelTimerLocked()
	service.mu.Unlock()
}

func TestVeDichVideoStopHoldsThenFinishesAndMatchPauseFreezesClock(t *testing.T) {
	service, _ := bellPolicyService(t, controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.20.0")
	stopVideoSchedule(t, service)
	ctx, must := context.Background(), bellApply(t)
	service.state.CurrentQuestion.Type, service.state.CurrentQuestion.AssetID = "video", "clip"
	state := must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(*service.state), Action: "start"}, Actor{}))
	time.Sleep(15 * time.Millisecond)
	state = must(service.Transition(ctx, StatusPaused, bellCommand(state), Actor{}))
	if video := state.Controller.VeDichVideo; video.StartedAt != 0 || video.PositionMS < 15 || video.Status != controller.VeDichVideoPlaying {
		t.Fatalf("match pause did not freeze the video clock: %+v", video)
	}
	state = must(service.Transition(ctx, StatusPlaying, bellCommand(state), Actor{}))
	if state.Controller.VeDichVideo.StartedAt == 0 {
		t.Fatal("match resume did not restart the video clock")
	}
	state = must(service.VeDichVideo(ctx, VeDichVideoCommand{Command: bellCommand(state), Action: "stop"}, Actor{}))
	if video := state.Controller.VeDichVideo; video.Status != controller.VeDichVideoHolding || video.StartedAt != 0 {
		t.Fatalf("stop = %+v", video)
	}
	deadline := time.Now().Add(4 * time.Second)
	for service.Current(parityAdminPrincipal).Controller.VeDichVideo.Status != controller.VeDichVideoIdle {
		if time.Now().After(deadline) {
			t.Fatal("server never finished after the stop hold")
		}
		time.Sleep(20 * time.Millisecond)
	}
	service.mu.Lock()
	service.cancelTimerLocked()
	service.mu.Unlock()
}
