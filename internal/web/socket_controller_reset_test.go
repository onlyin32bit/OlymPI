package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/controller"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

func TestControllerResetSocketRetainsExplicitEventAndStopsSound(t *testing.T) {
	ctx := context.Background()
	var database *store.Store
	_, games, _ := deploymentServerWithRepository(t, func(repository game.Repository) game.Repository {
		database = repository.(*store.Store)
		return repository
	})
	match, err := database.CreateMatch(ctx, game.Match{ID: "reset-match", Name: "Reset", Season: 1, Stage: "Final", Status: game.StatusPlaying}, game.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := games.SelectMatch(ctx, match.ID, game.Actor{})
	if err != nil {
		t.Fatal(err)
	}
	beforeVersion := state.Version
	siteSettings, err := settings.Open(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	stops := make(chan sound.StopEvent, 2)
	sounds, err := sound.NewService(ctx, database, siteSettings, func(room, event string, data any) {
		if room == sound.OutputRoom && event == "Sound.Stopped" {
			stops <- data.(sound.StopEvent)
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &controllerSocket{service: games, sounds: sounds}
	if _, err := handler.Reset(&opsocket.Ctx{}, transitionRequest{}); err == nil {
		t.Fatal("controller reset allowed a caller without the admin role")
	} else {
		var operationError *opsocket.Error
		if !errors.As(err, &operationError) || operationError.Code != "FORBIDDEN" {
			t.Fatalf("controller reset changed role rejection: %v", err)
		}
	}
	select {
	case <-stops:
		t.Fatal("unauthorized reset stopped sound")
	default:
	}

	socket := opsocket.NewSocket()
	if err := socket.RegisterRequestController("Controller", handler); err != nil {
		t.Fatal(err)
	}
	principal := opsocket.Principal{Subject: "1", Roles: []string{string(auth.RoleAdmin)}}
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
			t.Errorf("shut down reset socket test server: %v", err)
		}
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(3 * time.Second):
			t.Error("reset socket test listener did not stop")
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
	for attempt := range 2 {
		state, _ = paritySocketRequest(t, conn, "Controller.Reset", transitionRequest{Command: paritySocketCommand(state)})
		if state.Version != beforeVersion+int64(attempt)+1 || !reflect.DeepEqual(state.Controller, controller.DefaultState()) {
			t.Fatalf("explicit default-to-default socket reset was suppressed: %+v", state)
		}
		select {
		case stop := <-stops:
			if !stop.All {
				t.Fatalf("controller reset did not stop every sound: %+v", stop)
			}
		case <-time.After(time.Second):
			t.Fatal("controller reset no longer stops sound")
		}
	}
	events, err := games.ListEvents(ctx, match.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("socket resets did not append exactly two explicit events: %+v", events)
	}
	for _, event := range events[1:] {
		var payload controller.State
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if event.Type != "Controller.Reset" || !reflect.DeepEqual(payload, controller.DefaultState()) || event.State == nil || !reflect.DeepEqual(event.State.Controller, controller.DefaultState()) {
			t.Fatalf("reset event identity, controller payload, or snapshot changed: %+v", event)
		}
	}
}
