package game

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/settings"
)

type judgmentRepository struct {
	*bellPolicyRepository
	fail      bool
	duplicate bool
}

func (r *judgmentRepository) ListEvents(_ context.Context, _ string) ([]Event, error) {
	var events []Event
	for i, name := range r.events {
		state := cloneRuntime(r.snapshots[i])
		events = append(events, Event{Type: name, Payload: r.payloads[i], State: &state})
	}
	return events, nil
}
func (r *judgmentRepository) SaveRuntime(ctx context.Context, state RuntimeState, event string, payload any, actor Actor, id string) (RuntimeState, error) {
	if r.fail {
		return RuntimeState{}, errors.New("save failed")
	}
	if r.duplicate {
		return r.LoadRuntime(ctx, state.MatchID, state.Epoch)
	}
	return r.bellPolicyRepository.SaveRuntime(ctx, state, event, payload, actor, id)
}
func judgmentFixture(t *testing.T, screen controller.Screen, id string) (*Service, *judgmentRepository) {
	s, r := bellPolicyService(t, controller.PhaseKhoiDong, screen, id)
	wrapper := &judgmentRepository{bellPolicyRepository: r}
	s.repository = wrapper
	return s, wrapper
}
func judgmentRequest(s *Service, verdict string, claim int) KhoiDongJudgmentRequest {
	cmd := bellCommand(*s.state)
	cmd.CommandID = "unique"
	return KhoiDongJudgmentRequest{Command: cmd, QuestionID: s.state.Controller.SelectedQuestionID, Verdict: verdict, ExpectedBellSlot: claim}
}
func TestKhoiDongJudgmentScoresAndTransitions(t *testing.T) {
	scoring := settings.KhoiDongScoring{IndividualCorrect: 17, IndividualWrong: -3, CommonCorrect: 13, CommonWrong: -9, CommonFloorZero: true}
	cases := []struct {
		name                        string
		screen                      controller.Screen
		id, verdict                 string
		claim, initial, slot, delta int
		next                        string
	}{
		{"individual correct", controller.ScreenQuesTS1, "kd_rieng.0.0", "correct", 0, 0, 1, 17, ""},
		{"individual wrong", controller.ScreenQuesTS1, "kd_rieng.0.0", "wrong", 0, 0, 1, -3, ""},
		{"common correct", controller.ScreenQuesChung, "kd_chung.0", "correct", 2, 4, 2, 13, "kd_chung.1"},
		{"common floor", controller.ScreenQuesChung, "kd_chung.0", "wrong", 2, 4, 2, -4, "kd_chung.1"},
		{"common nobody", controller.ScreenQuesChung, "kd_chung.0", "wrong", 0, 4, 0, 0, "kd_chung.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := judgmentFixture(t, tc.screen, tc.id)
			if tc.claim != 0 {
				s.state.Contestants[tc.claim-1].HasRung = true
				s.state.Contestants[tc.claim-1].Score = tc.initial
			}
			s.state.Timer = TimerState{IsRunning: true, DurationMS: 3000, RemainingMS: 3000, StartedAt: time.Now().UnixMilli()}
			before := cloneRuntime(*s.state)
			result, err := s.JudgeKhoiDong(context.Background(), judgmentRequest(s, tc.verdict, tc.claim), scoring, Actor{})
			if err != nil {
				t.Fatal(err)
			}
			if result.Slot != tc.slot || result.Delta != tc.delta || result.Runtime.Controller.SelectedQuestionID != tc.next || !result.Runtime.Controller.IsShowingQuestion || result.Runtime.Timer.IsRunning || result.Runtime.InputEpoch != before.InputEpoch+1 || result.Runtime.BellEpoch != before.BellEpoch+1 {
				t.Fatalf("bad result: %+v", result)
			}
			for _, contestant := range result.Runtime.Contestants {
				if contestant.HasRung || contestant.BellLocked {
					t.Fatalf("bell not reset: %+v", contestant)
				}
			}
			if tc.slot != 0 && result.Runtime.Contestants[tc.slot-1].Score != tc.initial+tc.delta {
				t.Fatalf("incorrect score: %+v", result.Runtime.Contestants)
			}
			if (tc.next == "") != (result.Runtime.CurrentQuestion == nil) || len(repo.events) != 1 || repo.events[0] != "KhoiDong.Judged" {
				t.Fatalf("transition not persisted: %+v", result)
			}
		})
	}
}
func TestKhoiDongRejectsStaleAndInvalid(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Service, *KhoiDongJudgmentRequest)
	}{
		{"no bell", func(_ *Service, _ *KhoiDongJudgmentRequest) {}},
		{"stale version", func(_ *Service, r *KhoiDongJudgmentRequest) { r.Command.ExpectedVersion-- }},
		{"stale epoch", func(_ *Service, r *KhoiDongJudgmentRequest) { r.Command.ExpectedEpoch-- }},
		{"stale input", func(_ *Service, r *KhoiDongJudgmentRequest) { r.Command.ExpectedInputEpoch-- }},
		{"stale bell epoch", func(_ *Service, r *KhoiDongJudgmentRequest) { r.Command.ExpectedBellEpoch-- }},
		{"bell race", func(s *Service, _ *KhoiDongJudgmentRequest) { s.state.Contestants[1].HasRung = true }},
		{"question", func(_ *Service, r *KhoiDongJudgmentRequest) { r.QuestionID = "kd_chung.1" }},
		{"screen", func(s *Service, _ *KhoiDongJudgmentRequest) { s.state.Controller.Screen = controller.ScreenQuesTS1 }},
		{"hidden", func(s *Service, _ *KhoiDongJudgmentRequest) { s.state.Controller.IsShowingQuestion = false }},
		{"missing command", func(_ *Service, r *KhoiDongJudgmentRequest) { r.Command.CommandID = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := judgmentFixture(t, controller.ScreenQuesChung, "kd_chung.0")
			r := judgmentRequest(s, "correct", 0)
			tc.mutate(s, &r)
			if _, err := s.JudgeKhoiDong(context.Background(), r, settings.DefaultScoring().KhoiDong, Actor{}); err == nil {
				t.Fatal("expected rejection")
			}
			if len(repo.events) != 0 {
				t.Fatal("rejection persisted")
			}
		})
	}
}
func TestKhoiDongReplayConcurrencyAndRollback(t *testing.T) {
	s, repo := judgmentFixture(t, controller.ScreenQuesChung, "kd_chung.0")
	r := judgmentRequest(s, "wrong", 0)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.JudgeKhoiDong(context.Background(), r, settings.DefaultScoring().KhoiDong, Actor{})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 || len(repo.events) != 1 {
		t.Fatalf("success=%d events=%d", success, len(repo.events))
	}
	// A newly captured command for a manually reopened question remains rejected.
	s.state.Controller.SelectedQuestionID = "kd_chung.0"
	s.state.CurrentQuestion = &QuestionSnapshot{ID: "kd_chung.0", Content: "question"}
	r = judgmentRequest(s, "wrong", 0)
	r.Command.CommandID = "reopened"
	if _, err := s.JudgeKhoiDong(context.Background(), r, settings.DefaultScoring().KhoiDong, Actor{}); err == nil {
		t.Fatal("reopened question was judged")
	}

	s2, repo2 := judgmentFixture(t, controller.ScreenQuesChung, "kd_chung.0")
	s2.state.Timer = TimerState{IsRunning: true, DurationMS: 60000, RemainingMS: 60000, StartedAt: time.Now().UnixMilli()}
	s2.runTimerLocked(60000)
	before := cloneRuntime(*s2.state)
	repo2.fail = true
	if _, err := s2.JudgeKhoiDong(context.Background(), judgmentRequest(s2, "wrong", 0), settings.DefaultScoring().KhoiDong, Actor{}); err == nil {
		t.Fatal("expected save failure")
	}
	if s2.state.Timer != before.Timer || s2.timerCancel == nil || s2.state.Controller.SelectedQuestionID != before.Controller.SelectedQuestionID {
		t.Fatal("failure changed state or canceled timer")
	}
	repo2.fail, repo2.duplicate = false, true
	if _, err := s2.JudgeKhoiDong(context.Background(), judgmentRequest(s2, "wrong", 0), settings.DefaultScoring().KhoiDong, Actor{}); err == nil {
		t.Fatal("duplicate acknowledgement accepted")
	}
}

func TestKhoiDongBlocksSavedQuestionScoreUnlessUndone(t *testing.T) {
	cases := []struct {
		name, id, reason string
		screen           controller.Screen
		undo, blocked    bool
	}{
		{"individual saved", "kd_rieng.0.0", "Khởi động · TS1 câu 1", controller.ScreenQuesTS1, false, true},
		{"common batched", "kd_chung.0", "Tăng tốc · Câu 1, Khởi động chung · Câu 1", controller.ScreenQuesChung, false, true},
		{"individual undone", "kd_rieng.0.0", "Khởi động · TS1 câu 1", controller.ScreenQuesTS1, true, false},
		{"common undone", "kd_chung.0", "Khởi động chung · Câu 1, Tăng tốc · Câu 1", controller.ScreenQuesChung, true, false},
		{"unrelated", "kd_chung.0", "Khởi động chung · Câu 10", controller.ScreenQuesChung, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := judgmentFixture(t, tc.screen, tc.id)
			ctx := context.Background()
			cmd := bellCommand(*s.state)
			cmd.CommandID = "manual-save"
			if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 10}}, tc.reason, cmd, Actor{}); err != nil {
				t.Fatal(err)
			}
			if tc.undo {
				cmd = bellCommand(*s.state)
				cmd.CommandID = "manual-undo"
				if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: -10}}, "Hoàn tác · "+tc.reason, cmd, Actor{}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.JudgeKhoiDong(ctx, judgmentRequest(s, "wrong", 0), settings.DefaultScoring().KhoiDong, Actor{})
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v, err=%v", tc.blocked, err)
			}
		})
	}
}

// The verdict cue must start after the commit but before the next question is published:
// the sound leads the pause gap instead of trailing the question change.
func TestKhoiDongVerdictSoundStartsBeforeNextQuestionIsPublished(t *testing.T) {
	scoring := settings.KhoiDongScoring{IndividualCorrect: 10, IndividualWrong: -5}
	s, repo := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
	s.state.Timer = TimerState{IsRunning: true, DurationMS: 3000, RemainingMS: 3000, StartedAt: time.Now().UnixMilli()}
	var order []string
	s.publish = func(_, event string, _ any) {
		if event == "Match.PublicUpdated" {
			order = append(order, "publish")
		}
	}
	s.SetVerdictSound(func(verdict string, _ Actor) error {
		order = append(order, "sound:"+verdict)
		return nil
	})
	result, err := s.JudgeKhoiDong(context.Background(), judgmentRequest(s, "correct", 0), scoring, Actor{})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(order) != "[sound:correct publish]" || result.SoundWarning != "" {
		t.Fatalf("order=%v warning=%q; the verdict cue must start before the next question is published", order, result.SoundWarning)
	}
	if len(repo.events) != 1 {
		t.Fatalf("judgment not committed once: %v", repo.events)
	}
}

func TestKhoiDongVerdictSoundFailureKeepsJudgment(t *testing.T) {
	scoring := settings.KhoiDongScoring{IndividualCorrect: 10}
	s, repo := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
	s.SetVerdictSound(func(string, Actor) error { return errors.New("no output") })
	result, err := s.JudgeKhoiDong(context.Background(), judgmentRequest(s, "correct", 0), scoring, Actor{})
	if err != nil {
		t.Fatal(err)
	}
	if result.SoundWarning != verdictSoundFailed || result.Delta != 10 || len(repo.events) != 1 {
		t.Fatalf("failed cue must not undo the judgment: %+v", result)
	}
}
