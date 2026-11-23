package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/onlyin32bit/4t/internal/auth"
	"github.com/onlyin32bit/4t/internal/config"
	"github.com/onlyin32bit/4t/internal/game"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
	"github.com/onlyin32bit/4t/internal/settings"
	"github.com/onlyin32bit/4t/internal/sound"
	store "github.com/onlyin32bit/4t/internal/store/sqlite"
)

type displayTestServer struct {
	server   *Server
	settings *settings.Service
	addr     string
	cookie   *http.Cookie
}

func newDisplayTestServer(t *testing.T) displayTestServer {
	t.Helper()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "4t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	sessions := auth.NewService(database)
	if _, err := sessions.CreateAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	fixed, err := sessions.ListFixedContestants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	cfg.Server.Port = listener.Addr().(*net.TCPAddr).Port
	siteSettings, err := settings.Open(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	games, err := game.NewService(ctx, database, fixed, nil)
	if err != nil {
		t.Fatal(err)
	}
	sounds, err := sound.NewService(ctx, database, siteSettings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	socket := opsocket.NewSocket()
	server := New(cfg, sessions, siteSettings, games, sounds, socket)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.app.Listener(listener) }()
	t.Cleanup(func() {
		_ = server.app.ShutdownWithTimeout(2 * time.Second)
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(3 * time.Second):
			t.Error("test server did not stop")
		}
	})

	form := url.Values{"username": {"admin"}, "password": {"password123"}}
	login := mustRequest(t, http.MethodPost, "/api/login", strings.NewReader(form.Encode()))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := server.app.Test(login, -1)
	if err != nil || response.StatusCode != http.StatusOK || len(response.Cookies()) != 1 {
		t.Fatalf("login failed: %v", err)
	}
	return displayTestServer{server: server, settings: siteSettings, addr: listener.Addr().String(), cookie: response.Cookies()[0]}
}

func (d displayTestServer) dial(t *testing.T, query string) (*websocket.Conn, protocol.GreetingPayload) {
	t.Helper()
	header := http.Header{}
	header.Set("Cookie", d.cookie.Name+"="+d.cookie.Value)
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, response, err := dialer.Dial("ws://"+d.addr+"/ws"+query, header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var envelope protocol.Envelope
	if err := conn.ReadJSON(&envelope); err != nil || envelope.Operation != protocol.OperationGreeting {
		t.Fatalf("greeting: %v", err)
	}
	var greeting protocol.GreetingPayload
	_ = json.Unmarshal(envelope.Data, &greeting)
	return conn, greeting
}

func readEventNamed(t *testing.T, conn *websocket.Conn, name string) protocol.EventMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("waiting for %s: %v", name, err)
		}
		if envelope.Operation != protocol.OperationEvent {
			continue
		}
		var event protocol.EventMessage
		_ = json.Unmarshal(envelope.Data, &event)
		if event.Event == name {
			return event
		}
	}
}

func TestDisplayPathRedirectsToHashRoute(t *testing.T) {
	d := newDisplayTestServer(t)
	for _, path := range []string{"/display", "/display/"} {
		response, err := d.server.app.Test(mustRequest(t, http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusFound || response.Header.Get("Location") != "/#/display" {
			t.Fatalf("%s returned %d location=%q", path, response.StatusCode, response.Header.Get("Location"))
		}
	}
}

func TestPublicViewSocketAndSettingsUpdatedEvent(t *testing.T) {
	d := newDisplayTestServer(t)
	operator, operatorGreeting := d.dial(t, "")
	if !operatorGreeting.Authenticated {
		t.Fatal("operator socket lost its session")
	}
	display, displayGreeting := d.dial(t, "?view=public")
	if displayGreeting.Authenticated || displayGreeting.Subject != "" || len(displayGreeting.Roles) != 0 || displayGreeting.View != opsocket.ViewPublic {
		t.Fatalf("public view kept the operator session: %+v", displayGreeting)
	}

	values := d.settings.Get()
	values.EventName = "Đổi tên"
	body, _ := json.Marshal(values)
	update := mustRequest(t, http.MethodPut, "/api/settings", bytes.NewReader(body))
	update.Header.Set("Content-Type", "application/json")
	update.AddCookie(d.cookie)
	response, err := d.server.app.Test(update, -1)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("settings update returned %v %v", response.StatusCode, err)
	}
	for _, conn := range []*websocket.Conn{display, operator} {
		if event := readEventNamed(t, conn, SettingsUpdatedEvent); len(event.Data) != 0 {
			t.Fatalf("Settings.Updated must not carry a payload: %s", event.Data)
		}
	}
}
