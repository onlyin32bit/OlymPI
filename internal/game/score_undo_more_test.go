package game

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/settings"
)

func TestUndoRejectsMalformedAndCollidingHistory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		payloads []string
	}{
		{"bad delta", []string{`{"score_save_id":"x","adjustments":[{"slot":1,"delta":1001}]}`}},
		{"duplicate slots", []string{`{"score_save_id":"x","adjustments":[{"slot":1,"delta":10},{"slot":1,"delta":5}]}`}},
		{"duplicate id", []string{`{"score_save_id":"x","adjustments":[{"slot":1,"delta":10}]}`, `{"score_save_id":"x","adjustments":[{"slot":2,"delta":5}]}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
			for _, p := range tc.payloads {
				repo.events = append(repo.events, "Match.ScoresAdjusted")
				repo.payloads = append(repo.payloads, json.RawMessage(p))
				repo.snapshots = append(repo.snapshots, cloneRuntime(*s.state))
			}
			before := cloneRuntime(*s.state)
			n := len(repo.events)
			if _, err := s.UndoScores(context.Background(), "x", bellCommand(*s.state), Actor{}); err == nil {
				t.Fatal("malformed history accepted")
			}
			if !reflect.DeepEqual(before, *s.state) || len(repo.events) != n {
				t.Fatal("malformed history changed state")
			}
		})
	}
}

func TestLegacyUndoCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, reason    string
		judged, blocked bool
	}{
		{"before fast", "Hoàn tác · Khởi động · TS1 câu 1", false, false},
		{"other question", "Hoàn tác · Khởi động · TS1 câu 10", true, false},
		{"other round", "Hoàn tác · Tăng tốc · Câu 1", true, false},
		{"same question", "Hoàn tác · Tăng tốc · Câu 1, Khởi động · TS1 câu 1", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := judgmentFixture(t, controller.ScreenQuesTS1, "kd_rieng.0.0")
			ctx := context.Background()
			if tc.judged {
				if _, err := s.JudgeKhoiDong(ctx, judgmentRequest(s, "correct", 0), settings.DefaultScoring().KhoiDong, Actor{}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := s.AdjustScores(ctx, []ScoreAdjustment{{Slot: 1, Delta: -10}}, tc.reason, bellCommand(*s.state), Actor{})
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v: %v", tc.blocked, err)
			}
		})
	}
}
