package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
)

func TestListEventsAfterPreservesExclusiveOrderedHistoryAndSnapshots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, id := range []string{"first", "other"} {
		match, err := store.CreateMatch(ctx, game.Match{ID: id, Name: id, Season: 1, Stage: "Final", Status: game.StatusPlaying}, game.Actor{})
		if err != nil {
			t.Fatal(err)
		}
		state := game.RuntimeState{
			MatchID: id, Version: match.Version, Status: game.StatusPlaying,
			Controller:  controller.DefaultState(),
			Contestants: []game.ContestantState{{Slot: 1, Answer: "private response"}},
		}
		for sequence := 1; sequence <= 3; sequence++ {
			state.Contestants[0].Score = sequence * 10
			state, err = store.SaveRuntime(ctx, state, "Match.ScoreAdjusted", map[string]int{"delta": 10}, game.Actor{ClientID: "operator"}, fmt.Sprintf("%s-%d", id, sequence))
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	full, err := store.ListEvents(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 3 {
		t.Fatalf("full history has %d events, want 3", len(full))
	}
	for i, event := range full {
		if event.MatchID != "first" || event.Sequence != int64(i+1) || event.Type != "Match.ScoreAdjusted" || event.ClientID != "operator" {
			t.Fatalf("event identity/order changed: %+v", event)
		}
		if string(event.Payload) != `{"delta":10}` || event.State == nil || event.State.Version != int64(i+2) || event.State.Contestants[0].Score != (i+1)*10 || event.State.Contestants[0].Answer != "private response" {
			t.Fatalf("event payload or historical snapshot changed: %+v", event)
		}
	}

	for _, test := range []struct {
		after int64
		want  []game.Event
	}{
		{after: 0, want: full},
		{after: 1, want: full[1:]},
		{after: 2, want: full[2:]},
		{after: 3, want: []game.Event{}},
		{after: 99, want: []game.Event{}},
	} {
		t.Run(fmt.Sprintf("after_%d", test.after), func(t *testing.T) {
			got, err := store.ListEventsAfter(ctx, "first", test.after)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("after %d: got %+v, want %+v", test.after, got, test.want)
			}
			if len(got) == 0 {
				encoded, err := json.Marshal(got)
				if err != nil || string(encoded) != "[]" {
					t.Fatalf("empty catch-up must remain an array: %s, %v", encoded, err)
				}
			}
		})
	}
}
