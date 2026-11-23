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

func TestScoreUndoSingleUseAndFastJudgment(t *testing.T) {
	ctx := context.Background()
	s, repo := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
	cmd := bellCommand(*s.state)
	cmd.CommandID = "save"
	if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 10}, {Slot: 2, Delta: -5}}, "Khởi động · TS1 câu 1", cmd, Actor{}); err != nil {
		t.Fatal(err)
	}
	var save scoreSavePayload
	if err := json.Unmarshal(repo.payloads[len(repo.payloads)-1], &save); err != nil || save.ScoreSaveID != "save" {
		t.Fatalf("save payload: %+v %v", save, err)
	}
	// Unrelated score changes survive: the inverse comes from the event, not the current total.
	cmd = bellCommand(*s.state)
	cmd.CommandID = "other"
	if _, err := s.AdjustScore(ctx, 1, 7, "manual", cmd, Actor{}); err != nil {
		t.Fatal(err)
	}
	cmd = bellCommand(*s.state)
	cmd.CommandID = "undo"
	if _, err := s.UndoScores(ctx, "save", cmd, Actor{}); err != nil {
		t.Fatal(err)
	}
	if s.state.Contestants[0].Score != 7 || s.state.Contestants[1].Score != 0 {
		t.Fatalf("wrong inverse: %+v", s.state.Contestants)
	}
	var undo scoreSavePayload
	if err := json.Unmarshal(repo.payloads[len(repo.payloads)-1], &undo); err != nil || undo.UndoOf != "save" || undo.Reason != "Hoàn tác · Khởi động · TS1 câu 1" || undo.Adjustments[0].Delta != -10 || undo.Adjustments[1].Delta != 5 {
		t.Fatalf("undo payload: %+v %v", undo, err)
	}
	if _, err := s.JudgeKhoiDong(ctx, judgmentRequest(s, "correct", 0), settings.DefaultScoring().KhoiDong, Actor{}); err != nil {
		t.Fatal(err)
	}
	before := cloneRuntime(*s.state)
	n := len(repo.events)
	cmd = bellCommand(*s.state)
	cmd.CommandID = "stale-other-tab"
	if _, err := s.UndoScores(ctx, "save", cmd, Actor{}); err == nil {
		t.Fatal("duplicate undo accepted")
	}
	if !reflect.DeepEqual(before, *s.state) || len(repo.events) != n {
		t.Fatal("duplicate changed state")
	}
	cmd = bellCommand(*s.state)
	cmd.CommandID = "legacy"
	if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: -10}}, "Hoàn tác · Khởi động · TS1 câu 1", cmd, Actor{}); err == nil || !strings.Contains(err.Error(), "+/-") {
		t.Fatalf("legacy unsafe undo: %v", err)
	}
}

func TestScoreUndoRejectsInvalidAndRollsBack(t *testing.T) {
	ctx := context.Background()
	s, repo := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
	cmd := bellCommand(*s.state)
	cmd.CommandID = "save"
	if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 10}}, "other", cmd, Actor{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", " save", "missing"} {
		before := cloneRuntime(*s.state)
		n := len(repo.events)
		if _, err := s.UndoScores(ctx, id, bellCommand(*s.state), Actor{}); err == nil {
			t.Fatalf("accepted %q", id)
		}
		if !reflect.DeepEqual(before, *s.state) || len(repo.events) != n {
			t.Fatal("invalid undo changed state")
		}
	}
	before := cloneRuntime(*s.state)
	n := len(repo.events)
	repo.fail = true
	if _, err := s.UndoScores(ctx, "save", bellCommand(*s.state), Actor{}); err == nil {
		t.Fatal("save failure accepted")
	}
	if !reflect.DeepEqual(before, *s.state) || len(repo.events) != n {
		t.Fatal("failed undo changed state")
	}
}

func TestScoreCorrectionProvenanceAndUndo(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		kind                        string
		verdictFirst, undo, blocked bool
	}{
		{"correction", "correction", false, false, false},
		{"verdict", "verdict", false, false, true},
		{"legacy", "", false, false, true},
		{"undo correction after verdict", "correction", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s, _ := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
			label := "Khởi động · TS1 câu 1"
			if tc.verdictFirst {
				cmd := bellCommand(*s.state)
				cmd.CommandID = "verdict"
				if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 10}}, label, cmd, Actor{}, ScoreAdjustmentSource{QuestionID: "kd_rieng.0.0", Kind: "verdict"}); err != nil {
					t.Fatal(err)
				}
			}
			cmd := bellCommand(*s.state)
			cmd.CommandID = "save"
			var sources []ScoreAdjustmentSource
			if tc.kind != "" {
				sources = []ScoreAdjustmentSource{{QuestionID: "kd_rieng.0.0", Kind: tc.kind}}
			}
			if _, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: 5}}, label, cmd, Actor{}, sources...); err != nil {
				t.Fatal(err)
			}
			if tc.undo {
				cmd = bellCommand(*s.state)
				cmd.CommandID = "undo"
				if _, err := s.UndoScores(ctx, "save", cmd, Actor{}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.JudgeKhoiDong(ctx, judgmentRequest(s, "wrong", 0), settings.DefaultScoring().KhoiDong, Actor{})
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v: %v", tc.blocked, err)
			}
		})
	}
}
