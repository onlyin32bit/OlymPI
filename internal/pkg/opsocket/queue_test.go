package opsocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	clientws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
)

func serveSocket(t *testing.T, s *Socket, options AttachOptions) string {
	t.Helper()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	s.AttachToFiberWithOptions(app, "/ws", options)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- app.Listener(listener) }()
	t.Cleanup(func() {
		s.disconnectWhere(func(*Client) bool { return true })
		_ = app.ShutdownWithTimeout(2 * time.Second)
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(3 * time.Second):
			t.Error("websocket listener did not exit")
		}
	})
	return "ws://" + listener.Addr().String() + "/ws"
}

func dialSocket(t *testing.T, url string) (*clientws.Conn, protocol.GreetingPayload) {
	t.Helper()
	dialer := clientws.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, response, err := dialer.Dial(url, nil)
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
		t.Fatalf("read greeting: operation=%d error=%v", envelope.Operation, err)
	}
	var greeting protocol.GreetingPayload
	if err := json.Unmarshal(envelope.Data, &greeting); err != nil {
		t.Fatal(err)
	}
	return conn, greeting
}

func clientByID(s *Socket, id string) *Client {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	for _, client := range s.clients {
		if client.ID == id {
			return client
		}
	}
	return nil
}

func TestSlowClientCannotBlockPublisherOrOtherClients(t *testing.T) {
	s := NewSocket()
	s.sendQueueSize = 8
	s.writeWait = 500 * time.Millisecond
	url := serveSocket(t, s, AttachOptions{AllowAnonymous: true})
	slow, slowGreeting := dialSocket(t, url)
	_ = slow // never reads: its TCP window fills, then its queue
	fast, _ := dialSocket(t, url)

	const events = 64
	payload := json.RawMessage(`"` + strings.Repeat("x", 128*1024) + `"`)
	received := make(chan error, 1)
	progress := make(chan int, events)
	go func() {
		for i := range events {
			_ = fast.SetReadDeadline(time.Now().Add(5 * time.Second))
			var envelope protocol.Envelope
			if err := fast.ReadJSON(&envelope); err != nil {
				received <- fmt.Errorf("event %d: %w", i, err)
				return
			}
			var event protocol.EventMessage
			_ = json.Unmarshal(envelope.Data, &event)
			if want := fmt.Sprintf("Seq.%d", i); event.Event != want {
				received <- fmt.Errorf("out of order: got %s want %s", event.Event, want)
				return
			}
			progress <- i
		}
		received <- nil
	}()

	var slowest time.Duration
	read := -1
	for i := range events {
		// Pace to the healthy reader so only the stalled client falls behind.
		for read < i-2 {
			select {
			case read = <-progress:
			case <-time.After(5 * time.Second):
				t.Fatalf("fast client stalled at event %d", read)
			}
		}
		started := time.Now()
		s.baseSocket.EmitEvent(fmt.Sprintf("Seq.%d", i), payload)
		slowest = max(slowest, time.Since(started))
	}
	if slowest > 250*time.Millisecond {
		t.Fatalf("publisher blocked for %s", slowest)
	}
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fast client did not receive every event")
	}
	deadline := time.Now().Add(5 * time.Second)
	for clientByID(s, slowGreeting.ClientID) != nil {
		if time.Now().After(deadline) {
			t.Fatal("slow client was not disconnected")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestEnqueueOverflowDisconnectsOnlyThatClient(t *testing.T) {
	s := NewSocket()
	full := queuedClient(s, "public")
	other := queuedClient(s, "public")
	for range cap(full.send) {
		full.send <- []byte(`{}`)
	}
	s.EmitEventToRoom("public", "Ping", nil)
	if !full.closed {
		t.Fatal("overflowing client must be disconnected")
	}
	if other.closed || len(other.send) != 1 {
		t.Fatal("other clients must still receive the event")
	}
	if err := s.enqueue(full, []byte(`{}`)); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed client enqueue returned %v", err)
	}
}

func TestPublicViewDowngradesAuthenticatedConnection(t *testing.T) {
	s := NewSocket()
	admin := Principal{Subject: "1", Roles: []string{"admin"}}
	url := serveSocket(t, s, AttachOptions{
		AllowAnonymous:             true,
		RejectAuthenticationErrors: true,
		Authenticate: func(c *fiber.Ctx) (Principal, error) {
			if c.Query("expired") != "" {
				return Principal{}, errors.New("expired")
			}
			return admin, nil
		},
	})

	_, operator := dialSocket(t, url)
	if !operator.Authenticated || operator.View != "" {
		t.Fatalf("operator connection changed: %+v", operator)
	}
	conn, public := dialSocket(t, url+"?view=public")
	if public.Authenticated || public.Subject != "" || len(public.Roles) != 0 || public.View != ViewPublic {
		t.Fatalf("public view kept privileges: %+v", public)
	}
	client := clientByID(s, public.ClientID)
	if client == nil || !client.inRoom("public") || client.inRoom("role:admin") || client.Principal.Authenticated() {
		t.Fatal("public view must join only the public room as an anonymous principal")
	}
	// A stale operator cookie must not keep the audience display offline.
	_, stale := dialSocket(t, url+"?view=public&expired=1")
	if stale.Authenticated {
		t.Fatal("stale session upgraded the public view")
	}

	// Heartbeat probe echoes client_time with the server clock.
	if err := conn.WriteJSON(protocol.Envelope{Operation: protocol.OperationHeartbeat, Data: json.RawMessage(`{"client_time":1234}`)}); err != nil {
		t.Fatal(err)
	}
	var envelope protocol.Envelope
	if err := conn.ReadJSON(&envelope); err != nil || envelope.Operation != protocol.OperationHeartbeat {
		t.Fatalf("heartbeat reply: operation=%d error=%v", envelope.Operation, err)
	}
	var beat protocol.HeartbeatPayload
	_ = json.Unmarshal(envelope.Data, &beat)
	if beat.ClientTime != 1234 || beat.ServerTime <= 0 {
		t.Fatalf("heartbeat echo %+v", beat)
	}
}

func TestPublicViewRefusedWhereAnonymousIsNotAllowed(t *testing.T) {
	app := fiber.New()
	s := NewSocket()
	s.AttachToFiberWithOptions(app, "/ws", AttachOptions{
		Authenticate: func(*fiber.Ctx) (Principal, error) { return Principal{Subject: "1"}, nil },
	})
	response, err := app.Test(httptestRequest("/ws?view=public"))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("public view on authenticated-only socket returned %d", response.StatusCode)
	}
}

func TestServerHeartbeatReachesClients(t *testing.T) {
	s := NewSocket()
	s.pingEvery = 50 * time.Millisecond
	url := serveSocket(t, s, AttachOptions{AllowAnonymous: true})
	conn, _ := dialSocket(t, url)
	var envelope protocol.Envelope
	if err := conn.ReadJSON(&envelope); err != nil || envelope.Operation != protocol.OperationHeartbeat {
		t.Fatalf("expected periodic heartbeat: operation=%d error=%v", envelope.Operation, err)
	}
}

func httptestRequest(path string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "http://localhost:8090"+path, nil)
}
