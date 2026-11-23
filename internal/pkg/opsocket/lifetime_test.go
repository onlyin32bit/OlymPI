package opsocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	clientws "github.com/fasthttp/websocket"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
)

func TestRetiredClientCannotAccessReusedFiberWrapper(t *testing.T) {
	s := NewSocket()
	wrapper := &websocket.Conn{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := &Client{
		ID: "old-client", Ctx: ctx, Cancel: cancel, conn: wrapper,
		Principal: Principal{Subject: "old-subject"}, rooms: make(map[string]struct{}),
		send: make(chan []byte, 4),
	}
	old.join("private-room")
	s.clients[wrapper] = old
	s.EmitEventToRoom("private-room", "Private.Event", json.RawMessage(`{"secret":true}`))
	if len(old.send) != 1 {
		t.Fatal("room broadcast was not queued on the target client")
	}
	queued := <-old.send

	// Fiber can assign this exact wrapper address to an unrelated session after
	// its original handler returns. A nil embedded connection makes any stale
	// wrapper access fail deterministically, rather than relying on pool timing.
	old.closed = true
	cancel()
	replacement := &Client{
		ID: "replacement-client", Ctx: context.Background(), conn: wrapper,
		Principal: Principal{Subject: "replacement-subject"}, rooms: make(map[string]struct{}),
	}
	s.clients[wrapper] = replacement
	request := &Ctx{Socket: s.baseSocket, Conn: wrapper, client: old}
	if request.ClientID() != old.ID || request.Principal().Subject != "old-subject" || request.Context() != ctx {
		t.Fatal("retained request context switched to the replacement session")
	}
	for name, action := range map[string]func() error{
		"reply":             func() error { return request.ReplyOK("request", "Test.Operation", struct{}{}) },
		"event":             func() error { return request.EmitToThisConn("Private.Event", struct{}{}) },
		"queued room event": func() error { return s.enqueue(old, queued) },
		"heartbeat":         func() error { return s.enqueue(old, heartbeatFrame(1)) },
	} {
		if err := action(); !errors.Is(err, net.ErrClosed) {
			t.Errorf("%s on retired client returned %v, want net.ErrClosed", name, err)
		}
	}
	s.closeClient(old)
	if s.clients[wrapper] != replacement || replacement.closed {
		t.Fatal("stale close disconnected the replacement session")
	}
}

func TestConcurrentWebsocketDisconnectAndWritesRespectHandlerLifetime(t *testing.T) {
	s := NewSocket()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	s.AttachToFiberWithOptions(app, "/ws", AttachOptions{
		Authenticate: func(c *fiber.Ctx) (Principal, error) {
			return Principal{Subject: strings.Clone(c.Query("subject"))}, nil
		},
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- app.Listener(listener) }()
	t.Cleanup(func() {
		s.disconnectWhere(func(*Client) bool { return true })
		if err := app.ShutdownWithTimeout(2 * time.Second); err != nil {
			t.Errorf("shut down test server: %v", err)
		}
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(3 * time.Second):
			t.Error("websocket listener did not exit")
		}
	})
	dialer := clientws.Dialer{HandshakeTimeout: 3 * time.Second}
	message := protocol.Envelope{Operation: protocol.OperationEvent, Data: json.RawMessage(`{"eventType":"Test.Alive"}`)}
	var retired *Client
	for iteration := range 16 {
		subject := fmt.Sprintf("lifetime-%d", iteration)
		conn, response, err := dialer.Dial("ws://"+listener.Addr().String()+"/ws?subject="+subject, nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			t.Fatal(err)
		}
		var greeting protocol.Envelope
		if err := conn.ReadJSON(&greeting); err != nil || greeting.Operation != protocol.OperationGreeting {
			t.Fatalf("read greeting: operation=%d error=%v", greeting.Operation, err)
		}
		var current *Client
		s.clientsMutex.RLock()
		for _, client := range s.clients {
			if client.Principal.Subject == subject {
				current = client
				break
			}
		}
		s.clientsMutex.RUnlock()
		if current == nil {
			t.Fatal("upgraded websocket was not registered")
		}
		if retired != nil {
			if err := s.writeJSON(retired, message); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("retired session write returned %v", err)
			}
			if err := s.enqueue(retired, heartbeatFrame(1)); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("retired session heartbeat returned %v", err)
			}
			s.closeClient(retired)
		}
		if err := s.writeJSON(current, message); err != nil {
			t.Fatal(err)
		}
		var alive protocol.Envelope
		if err := conn.ReadJSON(&alive); err != nil || alive.Operation != protocol.OperationEvent {
			t.Fatalf("replacement session stopped receiving events: operation=%d error=%v", alive.Operation, err)
		}

		var operations sync.WaitGroup
		operations.Add(4)
		go func() {
			defer operations.Done()
			for range 8 {
				if err := s.writeJSON(current, message); err != nil {
					return
				}
			}
		}()
		go func() {
			defer operations.Done()
			for range 8 {
				if err := s.enqueue(current, heartbeatFrame(1)); err != nil {
					return
				}
			}
		}()
		go func() {
			defer operations.Done()
			s.DisconnectSubject(subject)
		}()
		go func() {
			defer operations.Done()
			_ = conn.Close()
		}()
		done := make(chan struct{})
		go func() { operations.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent close/write operations did not finish")
		}
		retired = current
	}
}
