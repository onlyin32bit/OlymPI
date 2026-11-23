package opsocket

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
)

type TestRequest struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

type TestResponse struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

type TestController struct {
	Called bool
}

func (tc *TestController) Test1(ctx *Ctx, req TestRequest) (TestResponse, error) {
	tc.Called = true
	return TestResponse{Field1: req.Field1, Field2: req.Field2}, nil
}

func (tc *TestController) Test2(ctx *Ctx, req struct{}) error {
	tc.Called = true
	return nil
}

func TestRegisterReqResController(t *testing.T) {
	s := NewSocket()
	s.RegisterRequestController("Timer", &TestController{})

	expected := map[string]controllerType{}
	v := reflect.ValueOf(&TestController{})
	expected["Timer.Test1"] = controllerType{
		name:     "Timer.Test1",
		handler:  v.MethodByName("Test1"),
		reqType:  reflect.TypeOf(TestRequest{}),
		respType: reflect.TypeOf(TestResponse{}),
	}
	expected["Timer.Test2"] = controllerType{
		name:     "Timer.Test2",
		handler:  v.MethodByName("Test2"),
		reqType:  reflect.TypeOf(struct{}{}),
		respType: nil,
	}

	for k, exp := range expected {
		c, ok := s.controllers[k]
		if !ok {
			t.Fatalf("missing controller %s", k)
		}
		if c.name != exp.name {
			t.Fatalf("controller name mismatch: got=%s expected=%s", c.name, exp.name)
		}
		if c.reqType != exp.reqType {
			t.Fatalf("req type mismatch for %s: got=%v expected=%v", k, c.reqType, exp.reqType)
		}
		if c.respType != exp.respType {
			t.Fatalf("resp type mismatch for %s: got=%v expected=%v", k, c.respType, exp.respType)
		}
	}
}

func TestHandleRequestInvokesController(t *testing.T) {
	s := NewSocket()
	ctrl := &TestController{}
	s.RegisterRequestController("Timer", ctrl)

	reqObj := TestRequest{Field1: "a", Field2: 2}
	b, _ := json.Marshal(reqObj)
	req := protocol.RequestPayload{ID: "1", Type: "Timer.Test1", Data: b}

	s.HandleRequest(&Ctx{Socket: s.baseSocket, Conn: nil}, req)

	if !ctrl.Called {
		t.Fatalf("expected controller to be called")
	}
}

func TestHandleRequestNotFound(t *testing.T) {
	s := NewSocket()
	ctrl := &TestController{}
	s.RegisterRequestController("Timer", ctrl)

	req := protocol.RequestPayload{ID: "1", Type: "Timer.NotFound", Data: []byte{}}
	s.HandleRequest(&Ctx{Socket: s.baseSocket, Conn: nil}, req)
}

func TestHandleRequestBadPayload(t *testing.T) {
	s := NewSocket()
	ctrl := &TestController{}
	s.RegisterRequestController("Timer", ctrl)

	req := protocol.RequestPayload{ID: "1", Type: "Timer.Test1", Data: []byte("invalid json")}
	s.HandleRequest(&Ctx{Socket: s.baseSocket, Conn: nil}, req)
}

func queuedClient(s *Socket, rooms ...string) *Client {
	conn := &websocket.Conn{}
	client := &Client{ID: "c", Ctx: context.Background(), conn: conn, rooms: make(map[string]struct{}), send: make(chan []byte, 8)}
	for _, room := range rooms {
		client.join(room)
	}
	s.clients[conn] = client
	return client
}

func readQueuedEvent(t *testing.T, client *Client) protocol.EventMessage {
	t.Helper()
	select {
	case frame := <-client.send:
		var env protocol.Envelope
		if err := json.Unmarshal(frame, &env); err != nil || env.Operation != protocol.OperationEvent {
			t.Fatalf("queued frame is not an event: %s (%v)", frame, err)
		}
		var ev protocol.EventMessage
		if err := json.Unmarshal(env.Data, &ev); err != nil {
			t.Fatalf("failed to unmarshal event message: %v", err)
		}
		return ev
	default:
		t.Fatal("no event queued")
	}
	return protocol.EventMessage{}
}

func TestEmitEventEnqueuesBroadcast(t *testing.T) {
	s := NewSocket()
	client := queuedClient(s)

	payload := map[string]string{"x": "y"}
	s.EmitEvent("greeting", payload)

	if ev := readQueuedEvent(t, client); ev.Event != "greeting" {
		t.Fatalf("event name mismatch: %s", ev.Event)
	}
}

func TestContextExtraction(t *testing.T) {
	s := NewSocket()
	s.clients = make(map[*websocket.Conn]*Client)

	conn := &websocket.Conn{}
	s.clients[conn] = &Client{ID: "test-id", Ctx: context.Background()}

	c := Ctx{Socket: s.baseSocket, Conn: conn}
	ctx := c.Context()

	if ctx == nil {
		t.Fatalf("expected context, got nil")
	}
}

func TestUnmarshalData(t *testing.T) {
	data := []byte(`{"field1":"hello","field2":42}`)

	c := Ctx{Data: json.RawMessage(data)}
	var result TestRequest
	err := c.Unmarshal(&result)

	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if result.Field1 != "hello" || result.Field2 != 42 {
		t.Fatalf("unmarshal mismatch: got %+v", result)
	}
}

func TestClientID(t *testing.T) {
	s := NewSocket()
	s.clients = make(map[*websocket.Conn]*Client)

	conn := &websocket.Conn{}
	s.clients[conn] = &Client{ID: "client-123"}

	c := Ctx{Socket: s.baseSocket, Conn: conn}
	id := c.ClientID()

	if id != "client-123" {
		t.Fatalf("expected 'client-123', got '%s'", id)
	}
}

func TestIntegrationRequestWithMultipleControllers(t *testing.T) {
	s := NewSocket()

	ctrl1 := &TestController{}
	ctrl2 := &TestController{}

	s.RegisterRequestController("Timer", ctrl1)
	s.RegisterRequestController("Controller", ctrl2)

	if len(s.controllers) < 2 {
		t.Fatalf("expected at least 2 controllers, got %d", len(s.controllers))
	}

	reqData := TestRequest{Field1: "test", Field2: 99}
	reqRaw, _ := json.Marshal(reqData)
	req := protocol.RequestPayload{
		ID:   "req1",
		Type: "Timer.Test1",
		Data: reqRaw,
	}

	s.HandleRequest(&Ctx{Socket: s.baseSocket, Conn: nil}, req)

	if !ctrl1.Called {
		t.Fatalf("expected Timer controller to be called")
	}
	if ctrl2.Called {
		t.Fatalf("expected Controller not to be called")
	}
}

func TestIntegrationBroadcastToAllClients(t *testing.T) {
	s := NewSocket()
	public := queuedClient(s, "public")
	admin := queuedClient(s, "role:admin")

	s.EmitEvent("broadcast_test", map[string]string{"msg": "broadcast"})
	for _, client := range []*Client{public, admin} {
		if ev := readQueuedEvent(t, client); ev.Event != "broadcast_test" {
			t.Fatalf("event name mismatch")
		}
	}

	s.EmitEventToRooms([]string{"public", "role:admin", "public"}, "Settings.Updated", nil)
	for _, client := range []*Client{public, admin} {
		ev := readQueuedEvent(t, client)
		if ev.Event != "Settings.Updated" || len(ev.Data) != 0 {
			t.Fatalf("unexpected room event %+v", ev)
		}
		if len(client.send) != 0 {
			t.Fatal("multi-room event must be delivered once per client")
		}
	}
}

func TestAttachRejectsForeignOriginAndInvalidAuthentication(t *testing.T) {
	newApp := func() *fiber.App {
		app := fiber.New()
		socket := NewSocket()
		socket.AttachToFiberWithOptions(app, "/ws", AttachOptions{
			AllowAnonymous:             true,
			RejectAuthenticationErrors: true,
			Authenticate: func(*fiber.Ctx) (Principal, error) {
				return Principal{}, errors.New("expired session")
			},
		})
		return app
	}

	foreign := httptest.NewRequest(http.MethodGet, "http://localhost:8090/ws", nil)
	foreign.Header.Set("Origin", "https://evil.example")
	response, err := newApp().Test(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin returned %d, expected %d", response.StatusCode, http.StatusForbidden)
	}

	invalid := httptest.NewRequest(http.MethodGet, "http://localhost:8090/ws", nil)
	response, err = newApp().Test(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid authentication returned %d, expected %d", response.StatusCode, http.StatusUnauthorized)
	}
}
