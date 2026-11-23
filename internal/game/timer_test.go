package game

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
)

func TestTimerSegmentsUseFrozenRemainingAndAccruedElapsed(t *testing.T) {
	timer := TimerState{IsRunning: true, DurationMS: 10000, RemainingMS: 6000, ElapsedMS: 4000, StartedAt: 20000}
	if remaining := timerRemaining(timer, 22500); remaining != 3500 {
		t.Fatalf("snapshot extended timer: %d", remaining)
	}
	if elapsed := timerElapsed(timer, 22500); elapsed != 6500 {
		t.Fatalf("lost earlier running segment: %d", elapsed)
	}
	paused := frozenTimer(timer, 22500)
	paused.IsPaused = true
	if paused.DurationMS != 10000 || paused.RemainingMS != 3500 || paused.ElapsedMS != 6500 {
		t.Fatalf("wrong paused totals: %+v", paused)
	}
	if timerRemaining(paused, 90000) != 3500 || timerElapsed(paused, 90000) != 6500 {
		t.Fatal("paused time advanced")
	}
	resumed := paused
	resumed.IsPaused, resumed.IsRunning, resumed.StartedAt = false, true, 100000
	if timerRemaining(resumed, 101000) != 2500 || timerElapsed(resumed, 101000) != 7500 {
		t.Fatal("resumed timer lost accumulated elapsed")
	}
	if timerRemaining(resumed, 104000) != 0 || timerElapsed(resumed, 104000) != 10000 {
		t.Fatal("timer deadline was not clamped")
	}
}

func TestTimerClockSkewCannotProduceNegativeElapsed(t *testing.T) {
	timer := TimerState{IsRunning: true, DurationMS: 10000, RemainingMS: 10000, StartedAt: 20000}
	if timerRemaining(timer, 19000) != 10000 || timerElapsed(timer, 19000) != 0 {
		t.Fatal("backward clock produced invalid elapsed")
	}
}

func TestPauseAtDeadlineCannotStrandRunningTimer(t *testing.T) {
	service := &Service{state: &RuntimeState{Timer: TimerState{
		IsRunning: true, DurationMS: 10, RemainingMS: 10, StartedAt: time.Now().UnixMilli() - 20,
	}}}
	service.pauseTimerLocked()
	if service.state.Timer.IsRunning || service.state.Timer.IsPaused ||
		service.state.Timer.RemainingMS != 0 || service.state.Timer.ElapsedMS != 10 {
		t.Fatalf("expired timer was resurrected without a worker: %+v", service.state.Timer)
	}
}

func TestQuestionTimerDurationRules(t *testing.T) {
	for _, tc := range []struct {
		name    string
		phase   controller.Phase
		id      string
		seconds int
		wantMS  int64
	}{
		{"KD common", controller.PhaseKhoiDong, "kd_chung.0", 999, 3_000},
		{"KD individual", controller.PhaseKhoiDong, "kd_rieng.3.0", 999, 3_000},
		{"VCNV first row", controller.PhaseVCNV, "vcnv.rows.0", 999, 15_000},
		{"VCNV last row", controller.PhaseVCNV, "vcnv.rows.3", 1, 15_000},
		{"VCNV centre custom", controller.PhaseVCNV, "vcnv.center", 42, 42_000},
		{"VCNV centre absent", controller.PhaseVCNV, "vcnv.center", 0, 15_000},
		{"VCNV centre legacy negative", controller.PhaseVCNV, "vcnv.center", -1, 15_000},
		{"VCNV centre maximum", controller.PhaseVCNV, "vcnv.center", 600, 600_000},
		{"TT custom", controller.PhaseTangToc, "tt.0", 4, 4_000},
		{"TT absent", controller.PhaseTangToc, "tt.0", 0, 30_000},
		{"TT legacy negative", controller.PhaseTangToc, "tt.0", -1, 30_000},
		{"TT maximum", controller.PhaseTangToc, "tt.0", 600, 600_000},
		{"VD 20 points", controller.PhaseVeDich, "vd.0.20.0", 999, 15_000},
		{"VD 30 points", controller.PhaseVeDich, "vd.3.30.1", 1, 20_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := controller.DefaultState()
			state.Phase, state.SelectedQuestionID = tc.phase, tc.id
			duration, err := questionTimerDuration(state, &QuestionSnapshot{ID: tc.id, Time: tc.seconds})
			if err != nil || duration != tc.wantMS {
				t.Fatalf("duration = %d, %v; want %d", duration, err, tc.wantMS)
			}
		})
	}
}

func TestStartTimerRejectsInvalidQuestionWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		phase    controller.Phase
		selected string
		question *QuestionSnapshot
	}{
		{"missing selection", controller.PhaseTangToc, "", &QuestionSnapshot{ID: "tt.0"}},
		{"missing snapshot", controller.PhaseTangToc, "tt.0", nil},
		{"stale snapshot", controller.PhaseTangToc, "tt.1", &QuestionSnapshot{ID: "tt.0"}},
		{"KD question in TT", controller.PhaseTangToc, "kd_chung.0", &QuestionSnapshot{ID: "kd_chung.0"}},
		{"TT question in KD", controller.PhaseKhoiDong, "tt.0", &QuestionSnapshot{ID: "tt.0"}},
		{"TT question in VCNV", controller.PhaseVCNV, "tt.0", &QuestionSnapshot{ID: "tt.0"}},
		{"TT question in VD", controller.PhaseVeDich, "tt.0", &QuestionSnapshot{ID: "tt.0"}},
		{"non-round phase", controller.PhaseMain, "tt.0", &QuestionSnapshot{ID: "tt.0"}},
		{"malformed KD question", controller.PhaseKhoiDong, "kd_rieng.0", &QuestionSnapshot{ID: "kd_rieng.0"}},
		{"malformed VCNV question", controller.PhaseVCNV, "vcnv.other", &QuestionSnapshot{ID: "vcnv.other"}},
		{"unsupported VD points", controller.PhaseVeDich, "vd.0.10.0", &QuestionSnapshot{ID: "vd.0.10.0"}},
		{"TT exceeds maximum", controller.PhaseTangToc, "tt.0", &QuestionSnapshot{ID: "tt.0", Time: 601}},
		{"VCNV exceeds maximum", controller.PhaseVCNV, "vcnv.center", &QuestionSnapshot{ID: "vcnv.center", Time: 601}},
		{"TT overflow-sized time", controller.PhaseTangToc, "tt.0", &QuestionSnapshot{ID: "tt.0", Time: int(^uint(0) >> 1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := controller.DefaultState()
			ctrl.Phase, ctrl.SelectedQuestionID = tc.phase, tc.selected
			repository := &controllerEventRepository{}
			service := &Service{
				repository: repository,
				state: &RuntimeState{
					MatchID: "match", Status: StatusPlaying, Controller: ctrl,
					CurrentQuestion: tc.question, InputEpoch: 3,
				},
			}
			before := cloneRuntime(*service.state)
			_, err := service.StartTimer(context.Background(), TimerStart{DurationMS: 1_000}, Actor{})
			socketErr, ok := err.(*opsocket.Error)
			if !ok || socketErr.Code != "VALIDATION" {
				t.Fatalf("invalid question was not rejected: %v", err)
			}
			if !reflect.DeepEqual(*service.state, before) || len(repository.events) != 0 || service.timerGeneration != 0 {
				t.Fatal("rejected timer start mutated or persisted runtime")
			}
		})
	}
}
