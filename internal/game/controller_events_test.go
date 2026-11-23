package game

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
)

type controllerEventRepository struct {
	Repository
	events    []string
	payloads  []json.RawMessage
	snapshots []RuntimeState
}

func (r *controllerEventRepository) SaveRuntime(_ context.Context, state RuntimeState, event string, payload any, _ Actor, _ string) (RuntimeState, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return RuntimeState{}, err
	}
	r.events = append(r.events, event)
	state.Version++
	r.payloads = append(r.payloads, raw)
	r.snapshots = append(r.snapshots, cloneRuntime(state))
	return cloneRuntime(state), nil
}

func controllerEventService() (*Service, *controllerEventRepository, *int) {
	ctrl := controller.DefaultState()
	ctrl.Phase, ctrl.Screen = controller.PhaseTangToc, controller.ScreenQues
	ctrl.SelectedQuestionID = "tt.0"
	ctrl.IsShowingQuestion = true
	repository := &controllerEventRepository{}
	publications := new(int)
	service := &Service{
		repository: repository,
		publish:    func(_, _ string, _ any) { *publications += 1 },
		state: &RuntimeState{
			MatchID: "match", Epoch: 2, Version: 9, InputEpoch: 4, BellEpoch: 6,
			Status: StatusPlaying, Controller: ctrl,
			Timer:           TimerState{IsRunning: true, DurationMS: 60000, RemainingMS: 60000, StartedAt: time.Now().UnixMilli()},
			Contestants:     []ContestantState{{Slot: 1, Score: 20, Answer: "kept answer", TimeMS: 500, Locked: true, HasRung: true}},
			CurrentQuestion: &QuestionSnapshot{ID: "tt.0", Answer: "private answer"},
			UpdatedAt:       time.UnixMilli(123456),
		},
		timerGeneration: 7,
	}
	return service, repository, publications
}

func TestIdenticalNormalizedControllerUpdateHasNoSideEffects(t *testing.T) {
	for _, variant := range []string{"identical", "nil defaults", "empty defaults"} {
		t.Run(variant, func(t *testing.T) {
			service, repository, publications := controllerEventService()
			timerContext, cancel := context.WithCancel(context.Background())
			defer cancel()
			service.timerCancel = cancel
			before := cloneRuntime(*service.state)
			next := cloneController(before.Controller)
			switch variant {
			case "nil defaults":
				next.AnswerStatus = nil
				for i := range next.VeDichContestantsState {
					next.VeDichContestantsState[i].SelectedQuestionSet = nil
				}
			case "empty defaults":
				next.AnswerStatus = []bool{}
			}
			for attempt := range 2 {
				got, err := service.UpdateController(context.Background(), next, Command{ExpectedVersion: before.Version, CommandID: fmt.Sprintf("noop-%d", attempt)}, Actor{})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, before) || !reflect.DeepEqual(*service.state, before) {
					t.Fatalf("no-op changed timer, inputs, window, version, or snapshot: got %+v, want %+v", got, before)
				}
			}
			if len(repository.events) != 0 || *publications != 0 || service.timerGeneration != 7 {
				t.Fatalf("no-op persisted/broadcast/cancelled timer: events=%v publications=%d generation=%d", repository.events, *publications, service.timerGeneration)
			}
			select {
			case <-timerContext.Done():
				t.Fatal("no-op cancelled the active timer")
			default:
			}

			next = cloneController(before.Controller)
			next.IsShowingQuestion = false
			got, err := service.UpdateController(context.Background(), next, Command{ExpectedVersion: before.Version, CommandID: "real-change"}, Actor{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Version != before.Version+1 || got.Controller.IsShowingQuestion || !reflect.DeepEqual(repository.events, []string{"Controller.Updated"}) || *publications != 6 {
				t.Fatalf("real controller change was not persisted and broadcast: state=%+v events=%v publications=%d", got, repository.events, *publications)
			}
			if got.Timer != before.Timer || got.InputEpoch != before.InputEpoch || got.BellEpoch != before.BellEpoch+1 || !reflect.DeepEqual(got.Contestants, before.Contestants) {
				t.Fatalf("reveal-only change disturbed timer or input state: %+v", got)
			}
		})
	}
}

func TestControllerDefaultAnswerStatusIsNotEqualToJudgedAnswer(t *testing.T) {
	service, repository, publications := controllerEventService()
	service.state.Controller.AnswerStatus[0] = false
	next := cloneController(service.state.Controller)
	next.AnswerStatus = nil
	got, err := service.UpdateController(context.Background(), next, Command{}, Actor{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Controller.AnswerStatus[0] || got.Version != 10 || len(repository.events) != 1 || *publications != 6 {
		t.Fatalf("default answer normalization hid a real change: state=%+v events=%v publications=%d", got, repository.events, *publications)
	}
}

func TestExplicitSemanticCommandsStillEmitEvents(t *testing.T) {
	service, repository, publications := controllerEventService()
	ctx := context.Background()
	for _, test := range []struct {
		event string
		apply func() (RuntimeState, error)
	}{
		{event: "Controller.AnswerStatusChanged", apply: func() (RuntimeState, error) { return service.SetAnswerStatus(ctx, 1, true, Command{}, Actor{}) }},
		{event: "Controller.VCNVStarted", apply: func() (RuntimeState, error) { return service.SetVCNVStarted(ctx, false, Command{}, Actor{}) }},
		{event: "Contestant.LockChanged", apply: func() (RuntimeState, error) { return service.SetContestantLock(ctx, 1, true, Command{}, Actor{}) }},
		{event: "Match.ScoreAdjusted", apply: func() (RuntimeState, error) { return service.AdjustScore(ctx, 1, 10, "correct", Command{}, Actor{}) }},
		{event: "Timer.Reset", apply: func() (RuntimeState, error) { return service.ResetTimer(ctx, Command{}, Actor{}) }},
		{event: "Timer.Reset", apply: func() (RuntimeState, error) { return service.ResetTimer(ctx, Command{}, Actor{}) }},
	} {
		beforeVersion, beforeEvents, beforePublications := service.state.Version, len(repository.events), *publications
		got, err := test.apply()
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != beforeVersion+1 || len(repository.events) != beforeEvents+1 || repository.events[beforeEvents] != test.event || *publications != beforePublications+6 {
			t.Fatalf("semantic %s was suppressed: version=%d events=%v publications=%d", test.event, got.Version, repository.events, *publications)
		}
	}
}

func TestResetControllerAlwaysEmitsExplicitEvent(t *testing.T) {
	service, repository, publications := controllerEventService()
	before := cloneRuntime(*service.state)
	for attempt := range 2 {
		got, err := service.ResetController(context.Background(), Command{ExpectedVersion: before.Version + int64(attempt)}, Actor{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Controller, controller.DefaultState()) || got.Version != before.Version+int64(attempt)+1 || got.Timer != (TimerState{}) || got.CurrentQuestion != nil {
			t.Fatalf("reset did not preserve controller reset semantics: %+v", got)
		}
		if got.InputEpoch != before.InputEpoch+1 || got.Contestants[0].Score != before.Contestants[0].Score || got.Contestants[0].Answer != "" || got.Contestants[0].TimeMS != 0 || got.Contestants[0].Locked || got.Contestants[0].HasRung {
			t.Fatalf("reset lost scores or retained previous question inputs: %+v", got)
		}
	}
	if !reflect.DeepEqual(repository.events, []string{"Controller.Reset", "Controller.Reset"}) || *publications != 12 {
		t.Fatalf("explicit reset of default state was suppressed or mislabeled: events=%v publications=%d", repository.events, *publications)
	}
	got, err := service.UpdateController(context.Background(), controller.DefaultState(), Command{}, Actor{})
	if err != nil || got.Version != before.Version+2 || len(repository.events) != 2 || *publications != 12 {
		t.Fatalf("routine default synchronization should remain a no-op: state=%+v events=%v publications=%d err=%v", got, repository.events, *publications, err)
	}
}

func TestResetControllerPreservesPlayableAndCommandChecks(t *testing.T) {
	for _, scenario := range []string{"paused", "stale version", "stale epoch", "other match", "no match"} {
		t.Run(scenario, func(t *testing.T) {
			service, repository, publications := controllerEventService()
			var command Command
			switch scenario {
			case "paused":
				service.state.Status = StatusPaused
			case "stale version":
				command.ExpectedVersion = service.state.Version - 1
			case "stale epoch":
				command.ExpectedEpoch = service.state.Epoch - 1
			case "other match":
				command.MatchID = "other"
			case "no match":
				service.state = nil
			}
			var before *RuntimeState
			if service.state != nil {
				state := cloneRuntime(*service.state)
				before = &state
			}
			if _, err := service.ResetController(context.Background(), command, Actor{}); err == nil {
				t.Fatal("reset bypassed the existing playable/command guard")
			}
			if !reflect.DeepEqual(service.state, before) || len(repository.events) != 0 || *publications != 0 {
				t.Fatalf("rejected reset had side effects: state=%+v events=%v publications=%d", service.state, repository.events, *publications)
			}
		})
	}
}

func TestJudgmentEventsPreservePreciseActionAndLegacyControllerPayload(t *testing.T) {
	ctx := context.Background()
	type judgment struct {
		action  map[string]any
		changed bool
		apply   func(*Service) (RuntimeState, error)
	}
	for _, family := range []struct {
		name  string
		event string
		steps []judgment
	}{
		{
			name: "answer status", event: "Controller.AnswerStatusChanged",
			steps: []judgment{
				{action: map[string]any{"slot": 3, "correct": true}, apply: func(s *Service) (RuntimeState, error) { return s.SetAnswerStatus(ctx, 3, true, Command{}, Actor{}) }},
				{action: map[string]any{"slot": 2, "correct": false}, changed: true, apply: func(s *Service) (RuntimeState, error) { return s.SetAnswerStatus(ctx, 2, false, Command{}, Actor{}) }},
				{action: map[string]any{"slot": 2, "correct": false}, apply: func(s *Service) (RuntimeState, error) { return s.SetAnswerStatus(ctx, 2, false, Command{}, Actor{}) }},
				{action: map[string]any{"slot": 4, "correct": true}, apply: func(s *Service) (RuntimeState, error) { return s.SetAnswerStatus(ctx, 4, true, Command{}, Actor{}) }},
			},
		},
		{
			name: "VCNV row", event: "Controller.VCNVRowStateChanged",
			steps: []judgment{
				{action: map[string]any{"row": 3, "state": "u"}, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVRowState(ctx, 3, "u", Command{}, Actor{}) }},
				{action: map[string]any{"row": 2, "state": "c"}, changed: true, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVRowState(ctx, 2, "c", Command{}, Actor{}) }},
				{action: map[string]any{"row": 2, "state": "c"}, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVRowState(ctx, 2, "c", Command{}, Actor{}) }},
				{action: map[string]any{"row": 4, "state": "u"}, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVRowState(ctx, 4, "u", Command{}, Actor{}) }},
			},
		},
		{
			name: "VCNV solved", event: "Controller.VCNVSolved",
			steps: []judgment{
				{action: map[string]any{"solved": false}, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVSolved(ctx, false, Command{}, Actor{}) }},
				{action: map[string]any{"solved": true}, changed: true, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVSolved(ctx, true, Command{}, Actor{}) }},
				{action: map[string]any{"solved": true}, apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVSolved(ctx, true, Command{}, Actor{}) }},
			},
		},
	} {
		t.Run(family.name, func(t *testing.T) {
			service, repository, publications := controllerEventService()
			if family.name != "answer status" {
				service.state.Controller.Phase = controller.PhaseVCNV
				normalizeBell(service.state, time.Now().UnixMilli())
			}
			for i, step := range family.steps {
				before := cloneRuntime(*service.state)
				got, err := step.apply(service)
				if err != nil {
					t.Fatal(err)
				}
				if got.Version != before.Version+1 || len(repository.events) != i+1 || repository.events[i] != family.event || *publications != (i+1)*6 {
					t.Fatalf("judgment did not persist and broadcast precisely once: state=%+v events=%v publications=%d", got, repository.events, *publications)
				}
				if changed := !reflect.DeepEqual(before.Controller, got.Controller); changed != step.changed {
					t.Fatalf("test judgment changed=%v, want %v", changed, step.changed)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(repository.payloads[i], &fields); err != nil {
					t.Fatal(err)
				}
				wantAction, err := json.Marshal(step.action)
				if err != nil {
					t.Fatal(err)
				}
				if string(fields["action"]) != string(wantAction) {
					t.Fatalf("judgment action was ambiguous or changed: got %s, want %s", fields["action"], wantAction)
				}
				delete(fields, "action")
				legacyPayload, err := json.Marshal(got.Controller)
				if err != nil {
					t.Fatal(err)
				}
				var legacyFields map[string]json.RawMessage
				if err := json.Unmarshal(legacyPayload, &legacyFields); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(fields, legacyFields) {
					t.Fatalf("existing top-level controller fields changed: got %s, want %s", repository.payloads[i], legacyPayload)
				}
				var legacyController controller.State
				if err := json.Unmarshal(repository.payloads[i], &legacyController); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(legacyController, got.Controller) || !reflect.DeepEqual(repository.snapshots[i], got) {
					t.Fatalf("legacy controller decoding or runtime snapshot changed: payload=%s state=%+v", repository.payloads[i], got)
				}
			}
		})
	}
}

func TestControllerEventsWithoutActionRetainOriginalPayload(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name  string
		apply func(*Service) (RuntimeState, error)
	}{
		{name: "generic update", apply: func(s *Service) (RuntimeState, error) {
			next := cloneController(s.state.Controller)
			next.IsShowingQuestion = false
			return s.UpdateController(ctx, next, Command{}, Actor{})
		}},
		{name: "explicit reset", apply: func(s *Service) (RuntimeState, error) { return s.ResetController(ctx, Command{}, Actor{}) }},
		{name: "other semantic controller action", apply: func(s *Service) (RuntimeState, error) { return s.SetVCNVStarted(ctx, false, Command{}, Actor{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, repository, _ := controllerEventService()
			got, err := test.apply(service)
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(got.Controller)
			if err != nil {
				t.Fatal(err)
			}
			if len(repository.payloads) != 1 || string(repository.payloads[0]) != string(want) {
				t.Fatalf("unannotated controller event payload changed: got %s, want %s", repository.payloads, want)
			}
		})
	}
}
