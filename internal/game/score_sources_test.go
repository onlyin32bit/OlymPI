package game

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/settings"
)

func TestAdjustScoresRejectsStaleKhoiDongDrafts(t *testing.T) {
	const id = "kd_rieng.0.0"
	const label = "Khởi động · TS1 câu 1"
	cases := []struct {
		name    string
		reason  string
		sources func(int64, int64) []ScoreAdjustmentSource
		blocked bool
		stale   bool
	}{
		{"two tabs", label, func(before, _ int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: before, Kind: "correction"}}
		}, true, true},
		{"unknown correction provenance", label, func(_, _ int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Kind: "correction"}}
		}, true, true},
		{"legacy", label, nil, true, true},
		{"old client post judgment", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after}}
		}, true, true},
		{"reopened card verdict", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "verdict"}}
		}, true, true},
		{"missing source in mixed batch", "Tăng tốc · Câu 1, " + label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: "kd_chung.1", Version: after}}
		}, true, true},
		{"unrelated source in mixed batch", label, func(before, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: "kd_chung.1", Version: after}, {QuestionID: id, Version: before, Kind: "correction"}}
		}, true, true},
		{"future version", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after + 1}}
		}, true, false},
		{"negative version", label, func(_, _ int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: -1}}
		}, true, false},
		{"malformed id", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: "kd_rieng.00.0", Version: after}}
		}, true, false},
		{"non KD id", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: "tt.0", Version: after}}
		}, true, false},
		{"invalid kind", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "manual"}}
		}, true, false},
		{"mixed verdict then correction", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "verdict"}, {QuestionID: id, Version: after, Kind: "correction"}}
		}, true, true},
		{"mixed correction then verdict", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "correction"}, {QuestionID: id, Version: after, Kind: "verdict"}}
		}, true, true},
		{"mixed omitted then correction", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after}, {QuestionID: id, Version: after, Kind: "correction"}}
		}, true, true},
		{"mixed correction then omitted", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "correction"}, {QuestionID: id, Version: after}}
		}, true, true},
		{"mixed old and new corrections", label, func(before, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "correction"}, {QuestionID: id, Version: before, Kind: "correction"}}
		}, true, true},
		{"duplicate post judgment corrections", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "correction"}, {QuestionID: id, Version: after, Kind: "correction"}}
		}, false, false},
		{"post judgment correction", label, func(_, after int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: after, Kind: "correction"}}
		}, false, false},
		{"exact label only", "Khởi động · TS1 câu 10", nil, false, false},
		{"manual reason", "Sửa điểm thủ công", nil, false, false},
		{"undo legacy", "Hoàn tác · " + label, nil, true, false},
		{"undo with old source", "Hoàn tác · " + label, func(before, _ int64) []ScoreAdjustmentSource {
			return []ScoreAdjustmentSource{{QuestionID: id, Version: before}}
		}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := judgmentFixture(t, controller.ScreenQuesTS1, id)
			ctx := context.Background()
			before := s.state.Version
			if _, err := s.JudgeKhoiDong(ctx, judgmentRequest(s, "correct", 0), settings.DefaultScoring().KhoiDong, Actor{}); err != nil {
				t.Fatal(err)
			}
			// Simulate returning to the same question after quick judgment.
			s.state.Controller.SelectedQuestionID = id
			s.state.CurrentQuestion = &QuestionSnapshot{ID: id, Content: "question"}
			after := s.state.Version
			var sources []ScoreAdjustmentSource
			if tc.sources != nil {
				sources = tc.sources(before, after)
			}
			prior := cloneRuntime(*s.state)
			eventCount := len(repo.events)
			cmd := bellCommand(*s.state)
			cmd.CommandID = "draft-save"
			_, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 10}, {Slot: 2, Delta: 5}}, tc.reason, cmd, Actor{}, sources...)
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v, error=%v", tc.blocked, err)
			}
			if tc.stale && (!strings.Contains(err.Error(), "bản nháp") || !strings.Contains(err.Error(), "+/-")) {
				t.Fatalf("unhelpful stale error: %v", err)
			}
			if tc.blocked && (!reflect.DeepEqual(prior, *s.state) || len(repo.events) != eventCount) {
				t.Fatalf("rejected batch changed state or events: %+v", s.state)
			}
			if !tc.blocked && (s.state.Contestants[0].Score != prior.Contestants[0].Score+10 || s.state.Contestants[1].Score != prior.Contestants[1].Score+5 || len(repo.events) != eventCount+1) {
				t.Fatal("valid batch was not applied atomically")
			}
		})
	}
}

func TestAdjustScoresCommonLegacyLabelAndUnknownSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name            string
		reason          string
		source          []ScoreAdjustmentSource
		missingSnapshot bool
		blocked         bool
	}{
		{"common legacy", "Tăng tốc · Câu 1, Khởi động chung · Câu 1", nil, false, true},
		{"common exact match", "Khởi động chung · Câu 10", nil, false, false},
		{"unknown event version", "Khởi động chung · Câu 1", []ScoreAdjustmentSource{{QuestionID: "kd_chung.0", Version: 11, Kind: "correction"}}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := judgmentFixture(t, controller.ScreenQuesChung, "kd_chung.0")
			ctx := context.Background()
			if _, err := s.JudgeKhoiDong(ctx, judgmentRequest(s, "wrong", 0), settings.DefaultScoring().KhoiDong, Actor{}); err != nil {
				t.Fatal(err)
			}
			if tc.missingSnapshot {
				repo.snapshots[0].Version = 0
			}
			before := cloneRuntime(*s.state)
			_, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 10}}, tc.reason, bellCommand(*s.state), Actor{}, tc.source...)
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v, error=%v", tc.blocked, err)
			}
			if tc.blocked && !reflect.DeepEqual(before, *s.state) {
				t.Fatal("rejected common draft changed state")
			}
		})
	}
}

func TestAdjustScoresUnknownSourceBeforeJudgment(t *testing.T) {
	s, _ := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
	_, err := s.AdjustScores(context.Background(), []ScoreAdjustment{{Slot: 1, Delta: 10}}, "Khởi động · TS1 câu 1", bellCommand(*s.state), Actor{}, ScoreAdjustmentSource{QuestionID: "kd_rieng.0.0"})
	if err != nil {
		t.Fatalf("unknown provenance without a judgment should remain saveable: %v", err)
	}
}

func TestScoreSourceJSONKindValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		json    string
		blocked bool
	}{
		{"omitted", `{"question_id":"kd_rieng.0.0","version":0}`, false},
		{"verdict", `{"question_id":"kd_rieng.0.0","version":0,"kind":"verdict"}`, false},
		{"correction", `{"question_id":"kd_rieng.0.0","version":0,"kind":"correction"}`, false},
		{"empty", `{"question_id":"kd_rieng.0.0","version":0,"kind":""}`, true},
		{"null", `{"question_id":"kd_rieng.0.0","version":0,"kind":null}`, true},
		{"unknown", `{"question_id":"kd_rieng.0.0","version":0,"kind":"manual"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
			var source ScoreAdjustmentSource
			if err := json.Unmarshal([]byte(tc.json), &source); err != nil {
				t.Fatal(err)
			}
			_, err := s.AdjustScores(context.Background(), []ScoreAdjustment{{Slot: 1, Delta: 10}}, "Khởi động · TS1 câu 1", bellCommand(*s.state), Actor{}, source)
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v, error=%v", tc.blocked, err)
			}
		})
	}
}
