package game_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

func timerPolicyMatch(t *testing.T) (*game.Service, game.RuntimeState) {
	t.Helper()
	ctx := context.Background()
	service, closeStore := newService(t, ctx)
	t.Cleanup(func() {
		if state := service.Current(parityAdmin); state.MatchID != "" {
			_, _ = service.ResetTimer(ctx, command(state), parityActor)
		}
		closeStore()
	})
	data := json.RawMessage(`{"kd_chung":[{"content":"Common question","time":99}],"kd_rieng":[[{"content":"Individual question","time":99}],[],[],[]],"vcnv":{"obstacle":"A","rows":[{"content":"First row","time":99},{"content":"Second row"},{"content":"Third row"},{"content":"Fourth row"}],"center":{"content":"Centre question","time":42}},"tt":[{"content":"One second","time":1},{"content":"Four seconds","time":4},{"content":"Default time"},{"content":"Legacy time","time":-1},{"content":"Maximum time","time":600},{"content":"Invalid time","time":601}],"vd":[{"20":[{"content":"20 points","time":99}],"30":[{"content":"30 points","time":99}]},{"20":[],"30":[]},{"20":[],"30":[]},{"20":[],"30":[]}]}`)
	set, err := service.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{Name: "Timer policy", Data: data}, parityActor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := service.CreateMatch(ctx, matchInput("Timer policy", set.ID), parityActor)
	if err != nil {
		t.Fatal(err)
	}
	must := mustRuntime(t)
	state := must(service.SelectMatch(ctx, match.ID, parityActor))
	state = must(service.Transition(ctx, game.StatusPlaying, command(state), parityActor))
	return service, state
}

func selectTimerQuestion(t *testing.T, service *game.Service, state game.RuntimeState, phase controller.Phase, screen controller.Screen, id string) game.RuntimeState {
	t.Helper()
	ctx, must := context.Background(), mustRuntime(t)
	ctrl := state.Controller
	ctrl.Phase, ctrl.Screen = phase, screen
	state = must(service.UpdateController(ctx, ctrl, command(state), parityActor))
	ctrl = state.Controller
	ctrl.SelectedQuestionID = id
	return must(service.UpdateController(ctx, ctrl, command(state), parityActor))
}

func TestTimerStartUsesGameOwnedDuration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		phase    controller.Phase
		screen   controller.Screen
		id       string
		legacyMS int64
		wantMS   int64
	}{
		{"KD common", controller.PhaseKhoiDong, controller.ScreenMainKD, "kd_chung.0", 60_000, 3_000},
		{"KD individual", controller.PhaseKhoiDong, controller.ScreenQuesTS1, "kd_rieng.0.0", 1, 3_000},
		{"VCNV row", controller.PhaseVCNV, controller.ScreenMainVCNV, "vcnv.rows.0", 99_000, 15_000},
		{"VCNV centre", controller.PhaseVCNV, controller.ScreenQues, "vcnv.center", 15_000, 42_000},
		{"TT configured", controller.PhaseTangToc, controller.ScreenQues, "tt.1", 1, 4_000},
		{"TT absent time", controller.PhaseTangToc, controller.ScreenIntro, "tt.2", 0, 30_000},
		{"TT legacy negative time", controller.PhaseTangToc, controller.ScreenQues, "tt.3", -1, 30_000},
		{"TT maximum", controller.PhaseTangToc, controller.ScreenQues, "tt.4", 600_001, 600_000},
		{"VD 20 points", controller.PhaseVeDich, controller.ScreenMainVD, "vd.0.20.0", 20_000, 15_000},
		{"VD 30 points", controller.PhaseVeDich, controller.ScreenQuesTS1, "vd.0.30.0", -600_000, 20_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, state := timerPolicyMatch(t)
			state = selectTimerQuestion(t, service, state, tc.phase, tc.screen, tc.id)
			if state.Controller.IsShowingQuestion {
				t.Fatal("fixture must exercise timer start without requiring a revealed question")
			}
			epoch := state.InputEpoch
			state = mustRuntime(t)(service.StartTimer(context.Background(), game.TimerStart{Command: command(state), DurationMS: tc.legacyMS}, parityActor))
			if !state.Timer.IsRunning || state.Timer.IsPaused || state.Timer.DurationMS != tc.wantMS ||
				state.Timer.RemainingMS != tc.wantMS || state.Timer.ElapsedMS != 0 || state.InputEpoch <= epoch {
				t.Fatalf("timer ignored game policy: %+v", state.Timer)
			}
		})
	}
}

func TestTimerStartRejectsInvalidSelectionAndStaleCommand(t *testing.T) {
	for _, scenario := range []string{"missing selection", "wrong phase", "configured time exceeds maximum", "stale command"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			service, state := timerPolicyMatch(t)
			state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "")
			captured := command(state)
			switch scenario {
			case "wrong phase":
				state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "kd_chung.0")
			case "configured time exceeds maximum":
				state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.5")
			case "stale command":
				state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.1")
			}
			cmd, wantCode := command(state), "VALIDATION"
			if scenario == "stale command" {
				cmd, wantCode = captured, "STALE_RUNTIME"
			}
			before := service.Current(parityAdmin)
			eventsBefore, err := service.ListEvents(ctx, state.MatchID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.StartTimer(ctx, game.TimerStart{Command: cmd, DurationMS: 1}, parityActor)
			socketErr, ok := err.(*opsocket.Error)
			if !ok || socketErr.Code != wantCode {
				t.Fatalf("invalid start error = %v, want %s", err, wantCode)
			}
			if after := service.Current(parityAdmin); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected timer start mutated runtime")
			}
			eventsAfter, err := service.ListEvents(ctx, state.MatchID)
			if err != nil || len(eventsAfter) != len(eventsBefore) {
				t.Fatalf("rejected timer start persisted events: %v", err)
			}
		})
	}
}

func TestTimerResetAndRestartRetainGameOwnedDuration(t *testing.T) {
	ctx, must := context.Background(), mustRuntime(t)
	service, state := timerPolicyMatch(t)
	state = selectTimerQuestion(t, service, state, controller.PhaseTangToc, controller.ScreenQues, "tt.1")
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state), DurationMS: 1}, parityActor))
	time.Sleep(20 * time.Millisecond)
	state = must(service.PauseTimer(ctx, command(state), parityActor))
	paused := state.Timer
	time.Sleep(20 * time.Millisecond)
	if current := service.Current(parityAdmin); current.Timer != paused {
		t.Fatal("paused timer advanced")
	}
	state = must(service.ResumeTimer(ctx, command(state), parityActor))
	if state.Timer.DurationMS != 4_000 || state.Timer.RemainingMS != paused.RemainingMS || state.Timer.ElapsedMS != paused.ElapsedMS {
		t.Fatalf("resume changed authoritative duration: %+v", state.Timer)
	}
	epoch := state.InputEpoch
	state = must(service.ResetTimer(ctx, command(state), parityActor))
	if state.Timer != (game.TimerState{}) || state.InputEpoch <= epoch {
		t.Fatalf("reset did not clear timer and invalidate its input window: %+v", state.Timer)
	}
	epoch = state.InputEpoch
	state = must(service.StartTimer(ctx, game.TimerStart{Command: command(state), DurationMS: -1}, parityActor))
	if state.Timer.DurationMS != 4_000 || state.Timer.RemainingMS != 4_000 || state.Timer.ElapsedMS != 0 || state.InputEpoch <= epoch {
		t.Fatalf("restart reused an elapsed segment or client duration: %+v", state.Timer)
	}
}

func TestTimerStartRetainsOptionalIgnoredLegacyJSONField(t *testing.T) {
	for _, raw := range []string{`{"command":{}}`, `{"command":{},"duration_ms":0}`, `{"command":{},"duration_ms":-1}`, `{"command":{},"duration_ms":600001}`} {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		var input game.TimerStart
		if err := decoder.Decode(&input); err != nil {
			t.Fatalf("cached timer input was rejected: %s: %v", raw, err)
		}
	}
	encoded, err := json.Marshal(game.TimerStart{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "duration_ms") {
		t.Fatalf("new timer inputs should omit unused duration: %s", encoded)
	}
}
