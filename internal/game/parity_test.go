package game_test

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
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

var parityAdmin = opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}}
var parityActor = game.Actor{UserID: 1}

func mustRuntime(t *testing.T) func(game.RuntimeState, error) game.RuntimeState {
	t.Helper()
	return func(state game.RuntimeState, err error) game.RuntimeState {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
}

func parityMatch(t *testing.T) (*game.Service, game.RuntimeState) {
	t.Helper()
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	t.Cleanup(func() { _, _ = service.ResetTimer(ctx, game.Command{}, parityActor); closeStore() })
	var data map[string]json.RawMessage
	if err := json.Unmarshal(validQuestions(), &data); err != nil {
		t.Fatal(err)
	}
	data["tt"] = json.RawMessage(`[{"content":"First question","answer":"PRIVATE FIRST","type":"text","time":60},{"content":"Second question","answer":"PRIVATE SECOND","type":"text","time":1}]`)
	data["vcnv"] = json.RawMessage(`{"obstacle":"A","rows":[{},{},{},{}],"center":{"content":"Center question","answer":"PRIVATE CENTER","type":"text"}}`)
	data["vd"] = json.RawMessage(`[{"20":[{"content":"TS1"}],"30":[]},{"20":[{"content":"TS2"}],"30":[]},{"20":[{"content":"TS3"}],"30":[]},{"20":[{"content":"TS4"}],"30":[]}]`)
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Parity", Data: encoded}, parityActor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := service.CreateMatch(ctx, matchInput("Parity", set.ID), parityActor)
	if err != nil {
		t.Fatal(err)
	}
	must := mustRuntime(t)
	state := must(service.SelectMatch(ctx, match.ID, parityActor))
	state = must(service.Transition(ctx, game.StatusPlaying, command(state), parityActor))
	ctrl := state.Controller
	ctrl.Phase, ctrl.Screen = controller.PhaseTangToc, controller.ScreenQues
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	ctrl = state.Controller
	ctrl.SelectedQuestionID = "tt.0"
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	ctrl = state.Controller
	ctrl.IsShowingQuestion = true
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	return service, state
}

func TestLastAnswerInTheWindowCountsAndMovesTheTime(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	state = must(service.SubmitAnswer(ctx, 1, "FIRST TRY", command(state), game.Actor{UserID: 2}))
	first := service.Current(parityAdmin).Contestants[0]
	if first.Answer != "FIRST TRY" || first.Locked {
		t.Fatalf("first answer was not recorded as editable: %+v", first)
	}
	time.Sleep(20 * time.Millisecond)
	state = must(service.SubmitAnswer(ctx, 1, "SECOND TRY", command(state), game.Actor{UserID: 2}))
	second := service.Current(parityAdmin).Contestants[0]
	if second.Answer != "SECOND TRY" || second.TimeMS <= first.TimeMS {
		t.Fatalf("the last answer did not replace the first one with a later time: first %+v second %+v", first, second)
	}
	if _, err := service.SubmitAnswer(ctx, 1, "   ", command(state), game.Actor{UserID: 2}); err == nil {
		t.Fatal("an empty answer replaced the recorded one")
	}
	state = must(service.SetContestantLock(ctx, 1, true, command(state), parityActor))
	if _, err := service.SubmitAnswer(ctx, 1, "AFTER LOCK", command(state), game.Actor{UserID: 2}); err == nil {
		t.Fatal("an operator lock did not block further answers")
	}
	if got := service.Current(parityAdmin).Contestants[0].Answer; got != "SECOND TRY" {
		t.Fatalf("the recorded answer changed after rejected input: %q", got)
	}
}

func TestIndependentContestantsShareInputWindowNotGlobalVersion(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	// All four clients capture the same state before any answer is accepted.
	captured := command(state)
	var group sync.WaitGroup
	errors := make(chan error, 4)
	for slot := 1; slot <= 4; slot++ {
		group.Add(1)
		go func(slot int) {
			defer group.Done()
			cmd := captured
			cmd.CommandID = fmt.Sprintf("independent-%d", slot)
			_, err := service.SubmitAnswer(ctx, slot, fmt.Sprintf("ANSWER %d", slot), cmd, game.Actor{UserID: int64(slot)})
			errors <- err
		}(slot)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("independent submission rejected: %v", err)
		}
	}
	current := service.Current(parityAdmin)
	for _, contestant := range current.Contestants {
		if contestant.Answer == "" || contestant.Locked {
			t.Fatalf("missing accepted input, or the answer locked further input: %+v", contestant)
		}
	}
	for _, principal := range []opsocket.Principal{{}, {Claims: map[string]string{"slot": "1"}}} {
		filtered := game.FilterRuntime(current, principal)
		if filtered.CurrentQuestion.Answer != "" {
			t.Fatal("official answer leaked in mutation snapshot")
		}
		for _, contestant := range filtered.Contestants {
			if contestant.Slot != 1 || principal.Claims == nil {
				if contestant.Answer != "" || contestant.TimeMS != 0 {
					t.Fatal("opponent answer leaked before review")
				}
			}
		}
	}
}

func TestServerRejectsAnswersOutsideWindow(t *testing.T) {
	for _, scenario := range []string{"no timer", "hidden question", "timer paused", "expired", "review screen", "wrong phase", "match paused"} {
		t.Run(scenario, func(t *testing.T) {
			service, state := parityMatch(t)
			ctx, must := context.Background(), mustRuntime(t)
			if scenario != "no timer" {
				if scenario == "expired" {
					ctrl := state.Controller
					ctrl.SelectedQuestionID = "tt.1"
					state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
					ctrl = state.Controller
					ctrl.IsShowingQuestion = true
					state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
				}
				state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
			}
			switch scenario {
			case "hidden question":
				ctrl := state.Controller
				ctrl.IsShowingQuestion = false
				state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			case "timer paused":
				state = must(service.PauseTimer(ctx, command(state), parityActor))
			case "match paused":
				state = must(service.Transition(ctx, game.StatusPaused, command(state), parityActor))
			case "expired":
				deadline := time.Now().Add(2 * time.Second)
				for {
					state = service.Current(parityAdmin)
					if !state.Timer.IsRunning {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("configured one-second timer did not expire")
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !state.Controller.IsShowingQuestion || state.Timer.DurationMS != 1_000 ||
					state.Timer.RemainingMS != 0 || state.Timer.ElapsedMS != 1_000 || state.Timer.IsPaused {
					t.Fatalf("expiry fixture did not finish the active timer: %+v", state.Timer)
				}
			case "review screen":
				ctrl := state.Controller
				ctrl.Screen = controller.ScreenAnswersTT
				state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			case "wrong phase":
				ctrl := state.Controller
				ctrl.Phase, ctrl.Screen = controller.PhaseKhoiDong, controller.ScreenQuesChung
				state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			}
			before := service.Current(parityAdmin)
			_, err := service.SubmitAnswer(ctx, 1, "TOO LATE", command(state), game.Actor{UserID: 2})
			if err == nil {
				t.Fatalf("accepted an answer with %s", scenario)
			}
			if after := service.Current(parityAdmin); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected input mutated runtime")
			}
		})
	}
}

func TestInputEpochRejectsPreviousWindowButAllowsUnrelatedUpdates(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	captured := command(state)
	state = must(service.AdjustScore(ctx, 4, 10, "unrelated", command(state), parityActor))
	captured.CommandID = "answer-after-score"
	state = must(service.SubmitAnswer(ctx, 1, "VALID", captured, game.Actor{UserID: 2}))
	state = must(service.ResetTimer(ctx, command(state), parityActor))
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	captured.CommandID = "old-window"
	if _, err := service.SubmitAnswer(ctx, 2, "OBSOLETE", captured, game.Actor{UserID: 3}); err == nil {
		t.Fatal("previous input window was accepted")
	}
	if _, err := service.SubmitAnswer(ctx, 2, "MISSING IDENTITY", game.Command{}, game.Actor{UserID: 3}); err == nil {
		t.Fatal("unscoped contestant command was accepted")
	}
	state = must(service.SubmitAnswer(ctx, 2, "CURRENT", command(state), game.Actor{UserID: 3}))
	if state.Contestants[0].Answer != "" {
		t.Fatal("timer restart retained a previous answer")
	}
}

func TestQuestionAndRoundChangesResetInputsButPreserveScores(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	state = must(service.AdjustScore(ctx, 1, 30, "correct", command(state), parityActor))
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	state = must(service.SubmitAnswer(ctx, 1, "OLD ANSWER", command(state), game.Actor{UserID: 2}))
	state = must(service.SetContestantLock(ctx, 2, true, command(state), parityActor))
	state = must(service.SetAnswerStatus(ctx, 1, false, command(state), parityActor))
	oldEpoch := state.InputEpoch
	ctrl := state.Controller
	ctrl.SelectedQuestionID = "tt.1"
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	assertFresh := func(state game.RuntimeState) {
		t.Helper()
		if state.Timer.IsRunning || state.Controller.IsShowingQuestion ||
			state.Controller.IsAllowingBell != (state.Controller.Phase == controller.PhaseVCNV) {
			t.Fatal("new question retained timer/reveal or has an incorrect automatic bell")
		}
		if state.Contestants[0].Score != 30 {
			t.Fatal("question reset changed score")
		}
		for _, c := range state.Contestants {
			if c.Answer != "" || c.Locked || c.BellLocked || c.HasRung || c.TimeMS != 0 {
				t.Fatalf("input was not reset: %+v", c)
			}
		}
		if !reflect.DeepEqual(state.Controller.AnswerStatus, []bool{true, true, true, true}) {
			t.Fatal("correctness not reset")
		}
	}
	assertFresh(state)
	if state.InputEpoch <= oldEpoch || state.CurrentQuestion.ID != "tt.1" {
		t.Fatal("question identity did not advance")
	}
	ctrl = state.Controller
	ctrl.Phase, ctrl.Screen = controller.PhaseVCNV, controller.ScreenMainVCNV
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	assertFresh(state)
	if state.CurrentQuestion != nil || state.Controller.SelectedQuestionID != "" {
		t.Fatal("round retained previous question")
	}
}

func TestAnswerReviewPublishesOnlyExplicitReviewAndPreservesInputs(t *testing.T) {
	for _, phase := range []controller.Phase{controller.PhaseTangToc, controller.PhaseVCNV} {
		t.Run(string(phase), func(t *testing.T) {
			service, state := parityMatch(t)
			ctx, must := context.Background(), mustRuntime(t)
			review := controller.ScreenAnswersTT
			if phase == controller.PhaseVCNV {
				ctrl := state.Controller
				ctrl.Phase, ctrl.Screen = phase, controller.ScreenQues
				state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
				ctrl = state.Controller
				ctrl.SelectedQuestionID = "vcnv.center"
				state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
				ctrl = state.Controller
				ctrl.IsShowingQuestion = true
				state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
				review = controller.ScreenAnswersVCNV
			}
			state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
			state = must(service.SubmitAnswer(ctx, 1, "PLAYER ONE", command(state), game.Actor{UserID: 2}))
			state = must(service.SubmitAnswer(ctx, 2, "PLAYER TWO", command(state), game.Actor{UserID: 3}))
			ctrl := state.Controller
			ctrl.Screen = review
			state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			state = must(service.SetAnswerStatus(ctx, 2, false, command(state), parityActor))
			public := service.Current(opsocket.Principal{})
			if public.Contestants[0].Answer != "PLAYER ONE" || public.Contestants[1].Answer != "PLAYER TWO" {
				t.Fatal("review did not publish responses")
			}
			if public.Controller.AnswerStatus[1] || public.Timer.IsRunning {
				t.Fatal("review correctness/timer state incorrect")
			}
			if public.CurrentQuestion != nil && public.CurrentQuestion.Answer != "" {
				t.Fatal("review leaked official answer before solution")
			}
			ctrl = state.Controller
			ctrl.Screen = controller.ScreenQues
			state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			if service.Current(opsocket.Principal{}).Contestants[0].Answer != "" {
				t.Fatal("leaving review retained public private responses")
			}
			if state.Contestants[0].Answer != "PLAYER ONE" {
				t.Fatal("review navigation destroyed saved answers")
			}
		})
	}
}

func TestVDDirectScreenNavigationSynchronizesSlot(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	ctrl := state.Controller
	ctrl.Phase, ctrl.Screen = controller.PhaseVeDich, controller.ScreenMainVD
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	for slot := 1; slot <= 4; slot++ {
		for _, prefix := range []string{"pre_ques_ts", "ques_ts"} {
			ctrl = state.Controller
			ctrl.Screen = controller.Screen(fmt.Sprintf("%s%d", prefix, slot))
			ctrl.SelectedContestant = 0
			state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			if state.Controller.SelectedContestant != slot {
				t.Fatalf("screen %s selected wrong slot", ctrl.Screen)
			}
		}
	}
}

func TestVeDichVideoControlsHoldThenStartQuestionTimer(t *testing.T) {
	for _, tc := range []struct {
		points string
		wantMS int64
	}{{"20", 15_000}, {"30", 20_000}} {
		t.Run(tc.points+" points", func(t *testing.T) {
			ctx := context.Background()
			service, closeStore := newService(t, ctx)
			t.Cleanup(func() {
				_, _ = service.ResetTimer(ctx, game.Command{}, parityActor)
				closeStore()
			})
			data := map[string]json.RawMessage{}
			if err := json.Unmarshal(validQuestions(), &data); err != nil {
				t.Fatal(err)
			}
			data["vd"] = json.RawMessage(`[{"20":[{"content":"Answer this in English","answer":"PRIVATE VIDEO ANSWER","type":"video","asset_id":"clip","time":99}],"30":[{"content":"Answer this in English","answer":"PRIVATE VIDEO ANSWER","type":"video","asset_id":"clip","time":99}]},{"20":[],"30":[]},{"20":[],"30":[]},{"20":[],"30":[]}]`)
			encoded, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Video finish", Data: encoded}, parityActor)
			if err != nil {
				t.Fatal(err)
			}
			match, err := service.CreateMatch(ctx, matchInput("Video finish", set.ID), parityActor)
			if err != nil {
				t.Fatal(err)
			}
			must := mustRuntime(t)
			state := must(service.SelectMatch(ctx, match.ID, parityActor))
			state = must(service.Transition(ctx, game.StatusPlaying, command(state), parityActor))
			ctrl := state.Controller
			ctrl.Phase, ctrl.Screen, ctrl.SelectedContestant = controller.PhaseVeDich, controller.ScreenQuesTS1, 1
			state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
			ctrl = state.Controller
			questionID := "vd.0." + tc.points + ".0"
			ctrl.SelectedQuestionID = questionID
			state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))

			state = must(service.VeDichVideo(ctx, game.VeDichVideoCommand{Command: command(state), Action: "start"}, parityActor))
			if !state.Controller.IsShowingQuestion || state.Controller.VeDichVideo.Status != controller.VeDichVideoPlaying {
				t.Fatalf("video start did not reveal the text prompt and video: %+v", state.Controller)
			}
			publicQuestion := service.Current(opsocket.Principal{})
			if publicQuestion.CurrentQuestion == nil || publicQuestion.CurrentQuestion.Content != "Answer this in English" || publicQuestion.CurrentQuestion.Answer != "" {
				t.Fatalf("public video question did not retain the prompt while hiding the answer: %+v", publicQuestion.CurrentQuestion)
			}
			if _, err := service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor); err == nil {
				t.Fatal("question timer started before the video finished")
			}
			state = must(service.VeDichVideo(ctx, game.VeDichVideoCommand{Command: command(state), Action: "pause"}, parityActor))
			if state.Controller.VeDichVideo.Status != controller.VeDichVideoPaused {
				t.Fatal("pause did not stop the video")
			}
			state = must(service.VeDichVideo(ctx, game.VeDichVideoCommand{Command: command(state), Action: "resume"}, parityActor))
			state = must(service.VeDichVideo(ctx, game.VeDichVideoCommand{Command: command(state), Action: "volume", Volume: 0.35}, parityActor))
			if state.Controller.VeDichVideo.Volume != 0.35 {
				t.Fatalf("volume change was not synchronized: %+v", state.Controller.VeDichVideo)
			}
			state = must(service.VeDichVideo(ctx, game.VeDichVideoCommand{Command: command(state), Action: "stop"}, parityActor))
			sessionID := state.Controller.VeDichVideo.SessionID
			if state.Controller.VeDichVideo.Status != controller.VeDichVideoHolding || state.Timer.IsRunning {
				t.Fatal("stop did not hold the final frame before the timer")
			}
			state = must(service.VeDichVideo(ctx, game.VeDichVideoCommand{
				Command: command(state), Action: "finish", QuestionID: questionID, SessionID: sessionID,
			}, parityActor))
			if state.Controller.VeDichVideo.Status != controller.VeDichVideoIdle || !state.Timer.IsRunning || state.Timer.DurationMS != tc.wantMS {
				t.Fatalf("video completion did not start the %s-point timer: controller=%+v timer=%+v", tc.points, state.Controller, state.Timer)
			}
			automaticDuration := state.Timer.DurationMS
			if state.Timer.Kind != game.TimerKindQuestion {
				t.Fatalf("video completion timer kind = %q", state.Timer.Kind)
			}
			replayed, err := service.VeDichVideo(ctx, game.VeDichVideoCommand{
				Command: command(state), Action: "finish", QuestionID: questionID, SessionID: sessionID,
			}, parityActor)
			if err != nil || replayed.Version != state.Version || replayed.Timer.StartedAt != state.Timer.StartedAt {
				t.Fatalf("duplicate video completion was not an idempotent no-op: err=%v timer=%+v", err, replayed.Timer)
			}
			if _, err := service.VeDichVideo(ctx, game.VeDichVideoCommand{
				Command: command(state), Action: "finish", QuestionID: questionID, SessionID: sessionID + 1,
			}, parityActor); err == nil {
				t.Fatal("completion for another video session was accepted")
			}
			state = must(service.ResetTimer(ctx, command(state), parityActor))
			state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state), DurationMS: 1}, parityActor))
			if state.Timer.DurationMS != automaticDuration {
				t.Fatalf("manual and automatic timer policies diverged: %+v", state.Timer)
			}
		})
	}
}

func TestResponseTimeAccumulatesAcrossPauseResume(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	time.Sleep(25 * time.Millisecond)
	state = must(service.PauseTimer(ctx, command(state), parityActor))
	elapsed, remaining, epoch := state.Timer.ElapsedMS, state.Timer.RemainingMS, state.InputEpoch
	if elapsed < 20 {
		t.Fatalf("pause lost elapsed time: %+v", state.Timer)
	}
	time.Sleep(25 * time.Millisecond)
	state = must(service.ResumeTimer(ctx, command(state), parityActor))
	if state.Timer.DurationMS != 60000 || state.Timer.ElapsedMS != elapsed || state.Timer.RemainingMS != remaining || state.InputEpoch != epoch {
		t.Fatal("resume changed total duration or input identity")
	}
	time.Sleep(25 * time.Millisecond)
	state = must(service.SubmitAnswer(ctx, 1, "RESUMED", command(state), game.Actor{UserID: 2}))
	if state.Contestants[0].TimeMS < elapsed+20 {
		t.Fatalf("answer lost pre-pause time: %+v", state.Contestants[0])
	}
}

func TestReplayedContestantCommandCannotRewindRuntime(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	captured := command(state)
	captured.CommandID = "original-answer"
	state = must(service.SubmitAnswer(ctx, 1, "ORIGINAL", captured, game.Actor{UserID: 2}))
	state = must(service.SetContestantLock(ctx, 1, false, command(state), parityActor))
	state = must(service.AdjustScore(ctx, 4, 30, "later score", command(state), parityActor))
	state = must(service.SubmitAnswer(ctx, 2, "LATER ANSWER", command(state), game.Actor{UserID: 3}))
	before := service.Current(parityAdmin)
	events, err := service.ListEvents(ctx, state.MatchID)
	if err != nil {
		t.Fatal(err)
	}
	replayed := must(service.SubmitAnswer(ctx, 1, "ORIGINAL", captured, game.Actor{UserID: 2}))
	if !reflect.DeepEqual(replayed, before) || !reflect.DeepEqual(service.Current(parityAdmin), before) {
		t.Fatal("duplicate input installed its historical snapshot or reapplied the answer")
	}
	after, err := service.ListEvents(ctx, state.MatchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(events) {
		t.Fatal("duplicate command persisted a second event")
	}
}

func TestVCNVBellsRemainIndependentOfAnswerWindows(t *testing.T) {
	service, state := parityMatch(t)
	ctx, must := context.Background(), mustRuntime(t)
	ctrl := state.Controller
	ctrl.Phase, ctrl.Screen = controller.PhaseVCNV, controller.ScreenQues
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	ctrl = state.Controller
	ctrl.SelectedQuestionID = "vcnv.center"
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	ctrl = state.Controller
	ctrl.IsShowingQuestion = true
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state)}, parityActor))
	captured := command(state)
	captured.CommandID = "bell-captured-before-answer-and-timer-reset"
	state = must(service.SubmitAnswer(ctx, 1, "STILL VALID", command(state), game.Actor{UserID: 2}))
	answerTime := state.Contestants[0].TimeMS
	state = must(service.Ring(ctx, 1, command(state), game.Actor{UserID: 2}))
	if state.Contestants[0].TimeMS != answerTime || !state.Controller.IsAllowingBell {
		t.Fatal("ring changed the submitted answer time or closed other bells")
	}
	state = must(service.ResetTimer(ctx, command(state), parityActor))
	state = must(service.Ring(ctx, 2, captured, game.Actor{UserID: 3}))
	if !state.Contestants[0].HasRung || !state.Contestants[1].HasRung ||
		state.Contestants[0].Answer != "STILL VALID" || state.BellEpoch != captured.ExpectedBellEpoch {
		t.Fatal("VCNV bell records or captured requests did not survive an answer-window change")
	}
}
