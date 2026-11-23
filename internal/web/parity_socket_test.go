package web

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
	"strings"
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

func TestContestantSocketAcknowledgementsRedactPrivateAnswers(t *testing.T) {
	for _, operation := range []string{"Contestant.SubmitAnswer", "Contestant.Ring"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			database, err := store.Open(ctx, filepath.Join(t.TempDir(), "parity.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := database.Close(); err != nil {
					t.Error(err)
				}
			})
			sessions := auth.NewService(database)
			if _, err := sessions.EnsureFixedContestants(ctx); err != nil {
				t.Fatal(err)
			}
			fixed, err := sessions.ListFixedContestants(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var slotOneID int64
			for _, user := range fixed {
				if user.FixedSlot == 1 {
					slotOneID = user.ID
				}
			}
			if slotOneID == 0 {
				t.Fatal("fixed contestant slot 1 was not created")
			}
			actor := game.Actor{UserID: slotOneID}
			admin := opsocket.Principal{Roles: []string{string(auth.RoleAdmin)}}
			games, err := game.NewService(ctx, database, fixed, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				current := games.Current(admin)
				if current.MatchID != "" {
					if _, err := games.ResetTimer(ctx, paritySocketCommand(current), actor); err != nil {
						t.Errorf("stop test timer: %v", err)
					}
				}
			})
			set, err := games.SaveQuestionSet(ctx, "", game.SaveQuestionSetInput{
				Name: "Socket privacy questions",
				Data: json.RawMessage(`{"kd_chung":[],"kd_rieng":[[],[],[],[]],"vcnv":{"obstacle":"A","rows":[{},{},{},{}],"center":{"content":"Public TT question","type":"text","answer":"OFFICIAL-TT-ANSWER-PRIVATE","keyword":"OFFICIAL-TT-KEYWORD-PRIVATE","time":60}},"tt":[{"content":"Public TT question","type":"text","answer":"OFFICIAL-TT-ANSWER-PRIVATE","keyword":"OFFICIAL-TT-KEYWORD-PRIVATE","time":60}],"vd":[]}`),
			}, actor)
			if err != nil {
				t.Fatal(err)
			}
			match, err := games.CreateMatch(ctx, game.CreateMatchInput{
				Name: "Socket privacy match", Season: 1, Stage: "Tứ kết", QuestionSetID: set.ID,
				Contestants: []game.Contestant{
					{Slot: 1, Name: "One", Class: "10A"},
					{Slot: 2, Name: "Two", Class: "10B"},
					{Slot: 3, Name: "Three", Class: "10C"},
					{Slot: 4, Name: "Four", Class: "10D"},
				},
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
			control := state.Controller
			control.Phase = controller.PhaseTangToc
			control.Screen = controller.ScreenQues
			if operation == "Contestant.Ring" {
				control.Phase = controller.PhaseVCNV
			}
			state, err = games.UpdateController(ctx, control, paritySocketCommand(state), actor)
			if err != nil {
				t.Fatal(err)
			}
			control = state.Controller
			control.SelectedQuestionID = "tt.0"
			if operation == "Contestant.Ring" {
				control.SelectedQuestionID = "vcnv.center"
			}
			state, err = games.UpdateController(ctx, control, paritySocketCommand(state), actor)
			if err != nil {
				t.Fatal(err)
			}
			control = state.Controller
			control.IsShowingQuestion = true
			state, err = games.UpdateController(ctx, control, paritySocketCommand(state), actor)
			if err != nil {
				t.Fatal(err)
			}
			state, err = games.StartTimer(ctx, game.TimerStart{Command: paritySocketCommand(state)}, actor)
			if err != nil {
				t.Fatal(err)
			}
			// Give the second contestant a genuinely nonzero private submission time.
			time.Sleep(2 * time.Millisecond)
			input := paritySocketCommand(state)
			input.ExpectedVersion = 0
			state, err = games.SubmitAnswer(ctx, 2, "OTHER-SLOT-ANSWER-PRIVATE", input, actor)
			if err != nil {
				t.Fatal(err)
			}
			if state.CurrentQuestion == nil || state.CurrentQuestion.Answer != "OFFICIAL-TT-ANSWER-PRIVATE" ||
				state.CurrentQuestion.Keyword != "OFFICIAL-TT-KEYWORD-PRIVATE" || state.Contestants[1].TimeMS <= 0 {
				t.Fatalf("privacy fixture did not contain secrets: %+v", state)
			}

			siteSettings, err := settings.Open(filepath.Join(t.TempDir(), "settings.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			sounds, err := sound.NewService(ctx, database, siteSettings, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			socket := opsocket.NewSocket()
			if err := RegisterSocketControllers(socket, games, sounds); err != nil {
				t.Fatal(err)
			}
			principal := opsocket.Principal{
				Subject: strconv.FormatInt(slotOneID, 10), Roles: []string{string(auth.RoleContestant)},
				Claims: map[string]string{"slot": "1"},
			}
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			socket.AttachToFiberWithOptions(app, "/socket", opsocket.AttachOptions{
				Authenticate: func(*fiber.Ctx) (opsocket.Principal, error) { return principal, nil },
			})
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			serveDone := make(chan error, 1)
			go func() { serveDone <- app.Listener(listener) }()
			t.Cleanup(func() {
				socket.DisconnectSubject(principal.Subject)
				if err := app.ShutdownWithTimeout(2 * time.Second); err != nil {
					t.Errorf("shut down websocket test server: %v", err)
				}
				_ = listener.Close()
				select {
				case <-serveDone:
				case <-time.After(3 * time.Second):
					t.Error("websocket test listener did not stop")
				}
			})
			dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
			conn, response, err := dialer.Dial("ws://"+listener.Addr().String()+"/socket", nil)
			if response != nil && response.Body != nil {
				_ = response.Body.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			input = paritySocketCommand(state)
			input.ExpectedVersion = 0
			var payload any = input
			expectedOwnAnswer := ""
			if operation == "Contestant.SubmitAnswer" {
				expectedOwnAnswer = "MY-SUBMITTED-ANSWER"
				payload = map[string]any{"command": input, "answer": "MY-SUBMITTED-ANSWER"}
			}
			ack, raw := paritySocketRequest(t, conn, operation, payload)
			for _, secret := range []string{"OFFICIAL-TT-ANSWER-PRIVATE", "OFFICIAL-TT-KEYWORD-PRIVATE", "OTHER-SLOT-ANSWER-PRIVATE"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("%s acknowledgement leaked %q: %s", operation, secret, raw)
				}
			}
			if ack.MatchID != state.MatchID || ack.Version <= state.Version || ack.CurrentQuestion == nil || ack.CurrentQuestion.Content != "Public TT question" {
				t.Fatalf("unexpected mutation acknowledgement: %+v", ack)
			}
			if len(ack.Contestants) != 4 || ack.Contestants[0].Answer != expectedOwnAnswer || ack.Contestants[0].TimeMS <= 0 {
				t.Fatalf("own result missing from acknowledgement: %+v", ack.Contestants)
			}
			for _, contestant := range ack.Contestants {
				if contestant.Slot != 1 && (contestant.Answer != "" || contestant.TimeMS != 0) {
					t.Fatalf("another contestant's answer/time leaked: %+v", contestant)
				}
			}
			if operation == "Contestant.Ring" && !ack.Contestants[0].HasRung {
				t.Fatal("ring acknowledgement did not retain own bell result")
			}
			if operation == "Contestant.SubmitAnswer" && ack.Contestants[0].Locked {
				t.Fatal("sending an answer locked further answers; the last answer in the window must count")
			}

			// Explicit review is the boundary that makes other contestants' results public.
			state = games.Current(admin)
			control = state.Controller
			control.Screen = controller.ScreenAnswersTT
			if operation == "Contestant.Ring" {
				control.Screen = controller.ScreenAnswersVCNV
			}
			state, err = games.UpdateController(ctx, control, paritySocketCommand(state), actor)
			if err != nil {
				t.Fatal(err)
			}
			review, _ := paritySocketRequest(t, conn, "Match.GetCurrent", struct{}{})
			if len(review.Contestants) != 4 || review.Contestants[1].Answer != "OTHER-SLOT-ANSWER-PRIVATE" ||
				review.Contestants[1].TimeMS != state.Contestants[1].TimeMS || review.Contestants[1].TimeMS <= 0 {
				t.Fatalf("explicit review did not reveal the other contestant's result: %+v", review.Contestants)
			}
			if review.Contestants[0].Answer != expectedOwnAnswer || review.Contestants[0].TimeMS != state.Contestants[0].TimeMS {
				t.Fatal("explicit review lost the requesting contestant's result")
			}
		})
	}
}

func paritySocketCommand(state game.RuntimeState) game.Command {
	return game.Command{
		MatchID: state.MatchID, ExpectedEpoch: state.Epoch, ExpectedVersion: state.Version,
		ExpectedInputEpoch: state.InputEpoch, ExpectedBellEpoch: state.BellEpoch, CommandID: uuid.NewString(),
	}
}

func paritySocketRequest(t *testing.T, conn *websocket.Conn, operation string, payload any) (game.RuntimeState, json.RawMessage) {
	t.Helper()
	requestID := uuid.NewString()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(protocol.RequestPayload{ID: requestID, Type: operation, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteJSON(protocol.Envelope{Operation: protocol.OperationRequest, Data: request}); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read %s acknowledgement: %v", operation, err)
		}
		if envelope.Operation != protocol.OperationResponse {
			continue
		}
		var response protocol.ResponsePayload
		if err := json.Unmarshal(envelope.Data, &response); err != nil {
			t.Fatal(err)
		}
		if response.ID != requestID || response.Type != operation {
			t.Fatalf("unexpected response: %+v", response)
		}
		if response.Status == nil || !response.Status.Ok {
			t.Fatalf("%s failed: %+v", operation, response.Status)
		}
		var runtime game.RuntimeState
		if err := json.Unmarshal(response.Data, &runtime); err != nil {
			t.Fatal(err)
		}
		return runtime, response.Data
	}
}
