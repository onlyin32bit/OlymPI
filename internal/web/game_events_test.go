package web

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func TestMatchEventsHTTPIncrementalHistory(t *testing.T) {
	ctx := context.Background()
	var database *store.Store
	server, games, _ := deploymentServerWithRepository(t, func(repository game.Repository) game.Repository {
		database = repository.(*store.Store)
		return repository
	})
	admin := deploymentLogin(t, server)
	contestant, err := database.CreateUser(ctx, "event-contestant", "unused", auth.RoleContestant)
	if err != nil {
		t.Fatal(err)
	}
	token := "event-contestant-session"
	sessions := auth.NewService(database)
	if err := database.CreateSession(ctx, contestant.ID, sessions.SessionID(token), "127.0.0.1", "test", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	contestantCookie := &http.Cookie{Name: sessionCookieName, Value: token}

	match, err := database.CreateMatch(ctx, game.Match{
		ID: "event-history", Name: "History", Season: 1, Stage: "Final", Status: game.StatusPlaying,
		Contestants: []game.Contestant{{Slot: 1, UserID: contestant.ID, Name: "Player", Class: "10A"}},
	}, game.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	ctrl := controller.DefaultState()
	ctrl.Phase, ctrl.Screen = controller.PhaseTangToc, controller.ScreenQues
	ctrl.SelectedQuestionID, ctrl.IsShowingQuestion = "tt.0", true
	state, err := database.SaveRuntime(ctx, game.RuntimeState{
		MatchID: match.ID, Version: match.Version, Status: match.Status, Controller: ctrl,
		Contestants:     []game.ContestantState{{Slot: 1, Answer: "private response", Score: 20}},
		CurrentQuestion: &game.QuestionSnapshot{Answer: "private answer key"},
	}, "Match.ScoreAdjusted", map[string]int{"delta": 20}, game.Actor{}, "seed")
	if err != nil {
		t.Fatal(err)
	}
	state, err = games.SelectMatch(ctx, match.ID, game.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	ctrl = state.Controller
	ctrl.IsShowingQuestion = false
	state, err = games.UpdateController(ctx, ctrl, game.Command{ExpectedVersion: state.Version, CommandID: "real-controller-change"}, game.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	beforeNoop := state
	ctrl = games.Controller(ctx)
	ctrl.IsAllowingBell = true // Cached manual flags must not create history events.
	ctrl.AnswerStatus = nil
	for i := range ctrl.VeDichContestantsState {
		ctrl.VeDichContestantsState[i].SelectedQuestionSet = nil
	}
	state, err = games.UpdateController(ctx, ctrl, game.Command{ExpectedVersion: state.Version, CommandID: "controller-noop"}, game.Actor{})
	if err != nil || !reflect.DeepEqual(state, beforeNoop) {
		t.Fatalf("no-op controller update changed persisted runtime: state=%+v, err=%v", state, err)
	}

	full, err := games.ListEvents(ctx, match.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 3 || full[2].Type != "Controller.Updated" || full[2].State == nil || full[2].State.Controller.IsShowingQuestion || full[2].State.Controller.IsAllowingBell {
		t.Fatalf("real controller change must append one history event: %+v", full)
	}
	path := "/api/matches/" + match.ID + "/events"
	for _, test := range []struct {
		query string
		start int
	}{
		{query: "", start: 0},
		{query: "?after_sequence=0", start: 0},
		{query: "?after_sequence=1", start: 1},
		{query: "?after_sequence=2", start: 2},
		{query: "?after_sequence=3", start: 3},
		{query: "?after_sequence=99", start: 3},
		{query: "?after_sequence=9223372036854775807", start: 3},
	} {
		t.Run("history"+test.query, func(t *testing.T) {
			body := deploymentRequest(t, server, http.MethodGet, path+test.query, nil, "", admin, http.StatusOK)
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope) != 1 || len(envelope["events"]) == 0 || envelope["events"][0] != '[' {
				t.Fatalf("history response shape changed: %s", body)
			}
			var events []game.Event
			if err := json.Unmarshal(envelope["events"], &events); err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(full[test.start:])
			if err != nil {
				t.Fatal(err)
			}
			if string(envelope["events"]) != string(want) {
				t.Fatalf("history cursor response differs from existing full history: got %s, want %s", envelope["events"], want)
			}
			for _, event := range events {
				if event.State == nil || event.State.CurrentQuestion == nil || event.State.CurrentQuestion.Answer != "private answer key" || event.State.Contestants[0].Answer != "private response" {
					t.Fatalf("admin history lost its full state snapshot: %+v", event)
				}
			}
		})
	}

	for _, query := range []string{
		"?after_sequence", "?after_sequence=", "?after_sequence=-1", "?after_sequence=bad",
		"?after_sequence=1.5", "?after_sequence=1e2", "?after_sequence=%20",
		"?after_sequence=9223372036854775808",
	} {
		t.Run("invalid"+query, func(t *testing.T) {
			deploymentRequest(t, server, http.MethodGet, path+query, nil, "", admin, http.StatusBadRequest)
		})
	}
	if _, err := games.ListEventsAfter(ctx, match.ID, -1); err == nil {
		t.Fatal("service accepted a negative history cursor")
	}

	for _, test := range []struct {
		name   string
		cookie *http.Cookie
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "contestant", cookie: contestantCookie, status: http.StatusForbidden},
	} {
		for _, query := range []string{"", "?after_sequence=2", "?after_sequence=-1"} {
			t.Run(test.name+query, func(t *testing.T) {
				deploymentRequest(t, server, http.MethodGet, path+query, nil, "", test.cookie, test.status)
			})
		}
	}
}
