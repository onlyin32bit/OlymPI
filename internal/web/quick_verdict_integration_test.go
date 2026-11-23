package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

// TestQuickVerdictSocketRehearsal crosses the real WebSocket, game service,
// SQLite transaction and sound service, not merely a mocked button handler.
func TestQuickVerdictSocketRehearsal(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "quick-verdict.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessions := auth.NewService(db)
	if _, err = sessions.EnsureFixedContestants(ctx); err != nil {
		t.Fatal(err)
	}
	fixed, err := sessions.ListFixedContestants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settingsService, err := settings.OpenWithRepository(ctx, filepath.Join(t.TempDir(), "settings.yaml"), db)
	if err != nil {
		t.Fatal(err)
	}
	values := settingsService.Get()
	values.Scoring.KhoiDong.IndividualCorrect = 15
	values.Scoring.KhoiDong.IndividualWrong = -3
	values.Scoring.KhoiDong.CommonCorrect = 17
	values.Scoring.KhoiDong.CommonWrong = -7
	values.Scoring.KhoiDong.CommonFloorZero = true
	if _, err = settingsService.Update(values); err != nil {
		t.Fatal(err)
	}

	socket := opsocket.NewSocket()
	games, err := game.NewService(ctx, db, fixed, func(room, event string, data any) {
		socket.EmitEventToRoom(room, event, data)
	})
	if err != nil {
		t.Fatal(err)
	}
	var soundsMu sync.Mutex
	var played []string
	soundService, err := sound.NewService(ctx, db, settingsService, func(room, event string, data any) {
		if event == "Sound.Played" {
			if playback, ok := data.(sound.Playback); ok {
				soundsMu.Lock()
				played = append(played, playback.SoundID)
				soundsMu.Unlock()
			}
		}
		socket.EmitEventToRoom(room, event, data)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = RegisterSocketControllers(socket, games, soundService, settingsService); err != nil {
		t.Fatal(err)
	}

	actor := game.Actor{UserID: 1}
	admin := opsocket.Principal{Subject: "1", Roles: []string{string(auth.RoleAdmin)}}
	set, err := games.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{
		Name: "Quick verdict questions",
		Data: json.RawMessage(`{"kd_chung":[{"content":"Common 1","type":"text"},{"content":"Common 2","type":"text"},{"content":"Common 3","type":"text"}],"kd_rieng":[[{"content":"Individual 1","type":"text"},{"content":"Individual 2","type":"text"}],[{"content":"Second contestant","type":"text"}],[{"content":"Third contestant","type":"text"}],[{"content":"Fourth contestant","type":"text"}]],"vcnv":{"obstacle":"A","rows":[{},{},{},{}],"center":{}},"tt":[],"vd":[]}`),
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	match, err := games.CreateMatch(ctx, game.CreateMatchInput{
		Name: "Quick verdict match", Season: 1, Stage: "Final", QuestionSetID: set.ID,
		Contestants: []game.Contestant{{Slot: 1, Name: "One", Class: "10A"}, {Slot: 2, Name: "Two", Class: "10B"}, {Slot: 3, Name: "Three", Class: "10C"}, {Slot: 4, Name: "Four", Class: "10D"}},
	}, actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err := games.SelectMatch(ctx, match.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = games.Transition(ctx, game.StatusPlaying, paritySocketCommand(state), actor)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	socket.AttachToFiberWithOptions(app, "/socket", opsocket.AttachOptions{
		Authenticate: func(c *fiber.Ctx) (opsocket.Principal, error) {
			if c.Query("role") == "admin" {
				return admin, nil
			}
			return opsocket.Principal{Subject: "2", Roles: []string{string(auth.RoleContestant)}, Claims: map[string]string{"slot": "2"}}, nil
		},
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- app.Listener(listener) }()
	t.Cleanup(func() {
		_ = app.ShutdownWithTimeout(2 * time.Second)
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(3 * time.Second):
			t.Error("socket server did not stop")
		}
	})
	dial := func(role string) *websocket.Conn {
		t.Helper()
		conn, response, dialErr := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial("ws://"+listener.Addr().String()+"/socket?role="+role, nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if dialErr != nil {
			t.Fatal(dialErr)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	adminConn, contestantConn := dial("admin"), dial("contestant")

	setQuestion := func(phase controller.Phase, screen controller.Screen, id string) game.RuntimeState {
		t.Helper()
		current := games.Current(admin)
		control := current.Controller
		control.Phase, control.Screen = phase, screen
		var updateErr error
		current, updateErr = games.UpdateController(ctx, control, paritySocketCommand(current), actor)
		if updateErr != nil {
			t.Fatal(updateErr)
		}
		control = current.Controller
		control.SelectedQuestionID = id
		current, updateErr = games.UpdateController(ctx, control, paritySocketCommand(current), actor)
		if updateErr != nil {
			t.Fatal(updateErr)
		}
		control = current.Controller
		control.IsShowingQuestion = true
		current, updateErr = games.UpdateController(ctx, control, paritySocketCommand(current), actor)
		if updateErr != nil {
			t.Fatal(updateErr)
		}
		return current
	}
	type judgment struct {
		Runtime      game.RuntimeState `json:"runtime"`
		QuestionID   string            `json:"question_id"`
		Verdict      string            `json:"verdict"`
		Slot         int               `json:"slot"`
		Delta        int               `json:"delta"`
		SoundWarning string            `json:"sound_warning"`
	}
	send := func(conn *websocket.Conn, operation string, payload any) protocol.ResponsePayload {
		t.Helper()
		data, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		requestID := uuid.NewString()
		encoded, marshalErr := json.Marshal(protocol.RequestPayload{ID: requestID, Type: operation, Data: data})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if writeErr := conn.WriteJSON(protocol.Envelope{Operation: protocol.OperationRequest, Data: encoded}); writeErr != nil {
			t.Fatal(writeErr)
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			var envelope protocol.Envelope
			if readErr := conn.ReadJSON(&envelope); readErr != nil {
				t.Fatal(readErr)
			}
			if envelope.Operation != protocol.OperationResponse {
				continue
			}
			var response protocol.ResponsePayload
			if unmarshalErr := json.Unmarshal(envelope.Data, &response); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			if response.ID != requestID {
				continue
			}
			return response
		}
	}
	request := func(conn *websocket.Conn, snapshot game.RuntimeState, questionID, verdict string, bell int, overrides ...game.Command) (judgment, *protocol.ResponseStatus) {
		t.Helper()
		command := paritySocketCommand(snapshot)
		if len(overrides) > 0 {
			command = overrides[0]
		}
		payload := map[string]any{"command": command, "question_id": questionID, "verdict": verdict, "expected_bell_slot": bell}
		response := send(conn, "Match.JudgeKhoiDong", payload)
		var result judgment
		if response.Status != nil && response.Status.Ok {
			if unmarshalErr := json.Unmarshal(response.Data, &result); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
		}
		return result, response.Status
	}
	assertOK := func(status *protocol.ResponseStatus) {
		t.Helper()
		if status == nil || !status.Ok {
			t.Fatalf("judgment rejected: %+v", status)
		}
	}
	assertRejected := func(status *protocol.ResponseStatus) {
		t.Helper()
		if status == nil || status.Ok {
			t.Fatalf("unsafe judgment was accepted: %+v", status)
		}
	}

	// A running timer is stopped, while the configured individual score is
	// committed in the same event that opens the next already-shown question.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesTS1, "kd_rieng.0.0")
	state, err = games.StartTimer(ctx, game.TimerStart{Command: paritySocketCommand(state)}, actor)
	if err != nil {
		t.Fatal(err)
	}
	before := state
	_, denied := request(contestantConn, state, "kd_rieng.0.0", "correct", 0)
	assertRejected(denied)
	firstCommand := paritySocketCommand(state)
	got, status := request(adminConn, state, "kd_rieng.0.0", "correct", 0, firstCommand)
	assertOK(status)
	if got.Slot != 1 || got.Delta != 15 || got.Runtime.Contestants[0].Score != 15 || got.Runtime.Controller.SelectedQuestionID != "kd_rieng.0.1" || !got.Runtime.Controller.IsShowingQuestion || got.Runtime.Timer.IsRunning {
		t.Fatalf("incorrect individual correct transition: %+v", got)
	}
	// A second operator's pending Q1 draft must not become a second award,
	// even though the save uses the latest match version after the fast action.
	staleDraft := map[string]any{
		"command":     paritySocketCommand(got.Runtime),
		"adjustments": []map[string]int{{"slot": 1, "delta": 15}},
		"reason":      "Khởi động · TS1 câu 1",
		"sources":     []map[string]any{{"question_id": "kd_rieng.0.0", "version": before.Version, "kind": "verdict"}},
	}
	assertRejected(send(adminConn, "Match.AdjustScores", staleDraft).Status)
	staleDraft["sources"] = []map[string]any{{"question_id": "kd_rieng.0.0", "version": before.Version, "kind": "correction"}}
	assertRejected(send(adminConn, "Match.AdjustScores", staleDraft).Status)
	delete(staleDraft, "sources") // an older client without provenance is safe too
	assertRejected(send(adminConn, "Match.AdjustScores", staleDraft).Status)
	if current := games.Current(admin); current.Contestants[0].Score != 15 {
		t.Fatalf("stale draft changed score: %+v", current.Contestants)
	}
	// A stale repeat must neither grade the old question twice nor the new one.
	_, status = request(adminConn, before, "kd_rieng.0.0", "correct", 0, firstCommand)
	assertRejected(status)
	got, status = request(adminConn, got.Runtime, "kd_rieng.0.1", "wrong", 0)
	assertOK(status)
	if got.Slot != 1 || got.Delta != -3 || got.Runtime.Contestants[0].Score != 12 || got.Runtime.Controller.SelectedQuestionID != "" || !got.Runtime.Controller.IsShowingQuestion || got.Runtime.CurrentQuestion != nil {
		t.Fatalf("incorrect last-question end card: %+v", got)
	}
	// Reopening a judged question is not permission to apply another award.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesTS1, "kd_rieng.0.0")
	_, status = request(adminConn, state, "kd_rieng.0.0", "correct", 0)
	assertRejected(status)

	// No bell: only Sai is valid; it changes no contestant's score.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesChung, "kd_chung.0")
	_, status = request(adminConn, state, "kd_chung.0", "correct", 0)
	assertRejected(status)
	got, status = request(adminConn, state, "kd_chung.0", "wrong", 0)
	assertOK(status)
	if got.Slot != 0 || got.Delta != 0 || got.Runtime.Controller.SelectedQuestionID != "kd_chung.1" || got.Runtime.Contestants[0].Score != 12 {
		t.Fatalf("incorrect unanswered transition: %+v", got)
	}
	// A late bell arriving after the operator captured 'no one rang' must
	// invalidate that click rather than silently score the wrong contestant.
	beforeBell := got.Runtime
	state, err = games.Ring(ctx, 2, paritySocketCommand(beforeBell), actor)
	if err != nil {
		t.Fatal(err)
	}
	_, status = request(adminConn, beforeBell, "kd_chung.1", "wrong", 0)
	assertRejected(status)
	got, status = request(adminConn, state, "kd_chung.1", "correct", 2)
	assertOK(status)
	if got.Slot != 2 || got.Delta != 17 || got.Runtime.Contestants[1].Score != 17 || got.Runtime.Contestants[1].HasRung || got.Runtime.Controller.SelectedQuestionID != "kd_chung.2" {
		t.Fatalf("incorrect bell-winner transition: %+v", got)
	}
	// Negative score is bounded by the configured floor, based on live score.
	state, err = games.AdjustScore(ctx, 3, 2, "floor fixture", paritySocketCommand(got.Runtime), actor)
	if err != nil {
		t.Fatal(err)
	}
	state, err = games.Ring(ctx, 3, paritySocketCommand(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	got, status = request(adminConn, state, "kd_chung.2", "wrong", 3)
	assertOK(status)
	if got.Slot != 3 || got.Delta != -2 || got.Runtime.Contestants[2].Score != 0 || got.Runtime.Controller.SelectedQuestionID != "" || !got.Runtime.Controller.IsShowingQuestion {
		t.Fatalf("incorrect floor or common end card: %+v", got)
	}

	soundsMu.Lock()
	actual := append([]string(nil), played...)
	soundsMu.Unlock()
	want := []string{"builtin.kd.correctanswer", "builtin.kd.wronganswer", "builtin.kd.wronganswer", "builtin.kd.correctanswer", "builtin.kd.wronganswer"}
	if fmt.Sprint(actual) != fmt.Sprint(want) {
		t.Fatalf("sound sequence = %v, want %v", actual, want)
	}
	events, err := games.ListEvents(ctx, match.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Type == "KhoiDong.Judged" {
			count++
		}
	}
	if count != len(want) {
		t.Fatalf("persisted %d judgments, want %d", count, len(want))
	}

	// Disabled audio cannot roll back a committed score or turn a success into
	// a retryable failure; a match pause, in contrast, rejects the judgment.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesTS2, "kd_rieng.1.0")
	state, err = games.Transition(ctx, game.StatusPaused, paritySocketCommand(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	_, status = request(adminConn, state, "kd_rieng.1.0", "correct", 0)
	assertRejected(status)
	state, err = games.Transition(ctx, game.StatusPlaying, paritySocketCommand(state), actor)
	if err != nil {
		t.Fatal(err)
	}
	values = settingsService.Get()
	values.Sound.Enabled = false
	if _, err = settingsService.Update(values); err != nil {
		t.Fatal(err)
	}
	got, status = request(adminConn, state, "kd_rieng.1.0", "correct", 0)
	assertOK(status)
	if got.Slot != 2 || got.Delta != 15 || got.Runtime.Contestants[1].Score != 32 {
		t.Fatalf("sound failure affected scoring: %+v", got)
	}
	if got.SoundWarning == "" {
		t.Fatal("committed judgment did not warn that its sound was unavailable")
	}
	soundsMu.Lock()
	soundCount := len(played)
	soundsMu.Unlock()
	if soundCount != len(want) {
		t.Fatalf("disabled sound unexpectedly played: %d sounds", soundCount)
	}

	// A newly created correction after fast judging is still permitted.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesTS2, "kd_rieng.1.0")
	repeatedCard := send(adminConn, "Match.AdjustScores", map[string]any{
		"command":     paritySocketCommand(state),
		"adjustments": []map[string]int{{"slot": 2, "delta": 15}},
		"reason":      "Khởi động · TS2 câu 1",
		"sources":     []map[string]any{{"question_id": "kd_rieng.1.0", "version": state.Version, "kind": "verdict"}},
	})
	assertRejected(repeatedCard.Status)
	correction := send(adminConn, "Match.AdjustScores", map[string]any{
		"command":     paritySocketCommand(state),
		"adjustments": []map[string]int{{"slot": 2, "delta": -5}},
		"reason":      "Khởi động · TS2 câu 1",
		"sources":     []map[string]any{{"question_id": "kd_rieng.1.0", "version": state.Version, "kind": "correction"}},
	})
	assertOK(correction.Status)
	var corrected game.RuntimeState
	if err = json.Unmarshal(correction.Data, &corrected); err != nil {
		t.Fatal(err)
	}
	if corrected.Contestants[1].Score != 27 {
		t.Fatalf("legitimate correction failed: %+v", corrected.Contestants)
	}

	// A duplicated tab must not undo the same manual save a second time after
	// the first tab has already undone it and fast-judged the question.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesTS3, "kd_rieng.2.0")
	saveCommand := paritySocketCommand(state)
	manual := send(adminConn, "Match.AdjustScores", map[string]any{
		"command":     saveCommand,
		"adjustments": []map[string]int{{"slot": 3, "delta": 10}},
		"reason":      "Khởi động · TS3 câu 1",
		"sources":     []map[string]any{{"question_id": "kd_rieng.2.0", "version": state.Version, "kind": "verdict"}},
	})
	assertOK(manual.Status)
	if err = json.Unmarshal(manual.Data, &state); err != nil {
		t.Fatal(err)
	}
	undo := send(adminConn, "Match.UndoScores", map[string]any{"command": paritySocketCommand(state), "undo_of": saveCommand.CommandID})
	assertOK(undo.Status)
	if err = json.Unmarshal(undo.Data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Contestants[2].Score != 0 {
		t.Fatalf("manual score was not undone: %+v", state.Contestants)
	}
	got, status = request(adminConn, state, "kd_rieng.2.0", "correct", 0)
	assertOK(status)
	if got.Runtime.Contestants[2].Score != 15 {
		t.Fatalf("fast judgment did not score: %+v", got)
	}
	staleUndo := send(adminConn, "Match.UndoScores", map[string]any{"command": paritySocketCommand(got.Runtime), "undo_of": saveCommand.CommandID})
	assertRejected(staleUndo.Status)
	legacyUndo := send(adminConn, "Match.AdjustScores", map[string]any{
		"command":     paritySocketCommand(got.Runtime),
		"adjustments": []map[string]int{{"slot": 3, "delta": -10}},
		"reason":      "Hoàn tác · Khởi động · TS3 câu 1",
	})
	assertRejected(legacyUndo.Status)
	if current := games.Current(admin); current.Contestants[2].Score != 15 {
		t.Fatalf("stale undo changed the fast score: %+v", current.Contestants)
	}

	// An explicitly typed correction while a question is live must not be
	// mistaken for a card verdict and block that question's later fast action.
	state = setQuestion(controller.PhaseKhoiDong, controller.ScreenQuesTS4, "kd_rieng.3.0")
	manualCorrection := send(adminConn, "Match.AdjustScores", map[string]any{
		"command":     paritySocketCommand(state),
		"adjustments": []map[string]int{{"slot": 4, "delta": 2}},
		"reason":      "Khởi động · TS4 câu 1",
		"sources":     []map[string]any{{"question_id": "kd_rieng.3.0", "version": state.Version, "kind": "correction"}},
	})
	assertOK(manualCorrection.Status)
	if err = json.Unmarshal(manualCorrection.Data, &state); err != nil {
		t.Fatal(err)
	}
	got, status = request(adminConn, state, "kd_rieng.3.0", "correct", 0)
	assertOK(status)
	if got.Runtime.Contestants[3].Score != 17 {
		t.Fatalf("a prior correction blocked or distorted the judgment: %+v", got)
	}
}
