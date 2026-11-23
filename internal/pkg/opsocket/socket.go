package opsocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
	applog "github.com/onlyin32bit/4t/internal/shared/logger"
)

type controllerType struct {
	name     string
	handler  reflect.Value
	reqType  reflect.Type
	respType reflect.Type
}

type OperationHandler func(*Ctx, json.RawMessage) (any, error)
type Middleware func(string, OperationHandler) OperationHandler
type Authenticator func(*fiber.Ctx) (Principal, error)

type AttachOptions struct {
	Authenticate               Authenticator
	AllowAnonymous             bool
	RejectAuthenticationErrors bool
	OriginAllowed              func(*fiber.Ctx, string) bool
	ValidateAuthenticated      func(context.Context, Principal) bool
	AuthCheckInterval          time.Duration
	ReadLimit                  int64
}

// ViewPublic is the `view` query value that downgrades a connection to the
// unauthenticated public principal (audience displays). It never upgrades.
const ViewPublic = "public"

type baseSocket struct {
	// builtInEvents
	clients      map[*websocket.Conn]*Client
	clientsMutex sync.RWMutex
	controllers  map[string]controllerType
	middleware   []Middleware
	debug        bool
	// Zero values use the defaults below; tests shrink them.
	sendQueueSize int
	writeWait     time.Duration
	pingEvery     time.Duration
}

const (
	writeWait     = 10 * time.Second
	pongWait      = 60 * time.Second
	pingEvery     = 25 * time.Second
	sendQueueSize = 256
)

func (s *baseSocket) writeTimeout() time.Duration {
	if s.writeWait > 0 {
		return s.writeWait
	}
	return writeWait
}

func (s *baseSocket) pingInterval() time.Duration {
	if s.pingEvery > 0 {
		return s.pingEvery
	}
	return pingEvery
}

func (s *baseSocket) queueSize() int {
	if s.sendQueueSize > 0 {
		return s.sendQueueSize
	}
	return sendQueueSize
}

// registerClient queues the greeting before the client becomes visible to
// broadcasts, so it is always the first frame on the connection.
func (s *baseSocket) registerClient(c *websocket.Conn, principal Principal, view string) *Client {
	ctx := context.WithValue(context.Background(), ConnCtxKey, c)
	ctx, cancel := context.WithCancel(ctx)
	client := &Client{
		ID: uuid.NewString(), IsAlive: true, Ctx: ctx, Cancel: cancel,
		Principal: principal, View: view, conn: c, rooms: make(map[string]struct{}),
		send: make(chan []byte, s.queueSize()),
	}
	if greeting, err := json.Marshal(greetingEnvelope(client, s.debug)); err == nil {
		client.send <- greeting
	}
	if !principal.Authenticated() {
		client.join("public")
	}
	for _, role := range principal.Roles {
		client.join("role:" + role)
	}
	if slot := principal.Claims["slot"]; slot != "" {
		client.join("contestant:" + slot)
	}
	s.clientsMutex.Lock()
	s.clients[c] = client
	s.clientsMutex.Unlock()
	s.debugf("connected client=%s remote=%s subject=%s roles=%v view=%s", client.ID, c.RemoteAddr(), principal.Subject, principal.Roles, view)
	return client
}

func (s *baseSocket) closeClient(client *Client) {
	if client == nil {
		return
	}
	// The handler's deferred close also takes this lock. It cannot return and
	// release Fiber's pooled wrapper while another goroutine still uses it.
	client.lifecycle.Lock()
	defer client.lifecycle.Unlock()
	if client.closed {
		return
	}
	client.closed = true
	s.debugf("disconnected client=%s", client.ID)
	client.IsAlive = false
	if client.Cancel != nil {
		client.Cancel()
	}
	if client.conn != nil && client.conn.Conn != nil {
		_ = client.conn.Close()
	}
	s.clientsMutex.Lock()
	if s.clients[client.conn] == client {
		delete(s.clients, client.conn)
	}
	s.clientsMutex.Unlock()
}

type Socket struct{ *baseSocket }

func NewSocket() *Socket {
	bs := &baseSocket{
		clients:     make(map[*websocket.Conn]*Client),
		controllers: make(map[string]controllerType),
	}
	return &Socket{bs}
}

func (s *Socket) Use(middleware ...Middleware) { s.middleware = append(s.middleware, middleware...) }

func (s *Socket) SetDebug(enabled bool) { s.debug = enabled }

func (s *Socket) DisconnectSubject(subject string) {
	s.disconnectWhere(func(client *Client) bool { return client.Principal.Subject == subject })
}

func (s *Socket) DisconnectSession(sessionID string) {
	s.disconnectWhere(func(client *Client) bool { return client.Principal.Claims["session_id"] == sessionID })
}

func (s *Socket) disconnectWhere(matches func(*Client) bool) {
	s.clientsMutex.RLock()
	connections := make([]*Client, 0)
	for _, client := range s.clients {
		if matches(client) {
			connections = append(connections, client)
		}
	}
	s.clientsMutex.RUnlock()
	for _, connection := range connections {
		s.closeClient(connection)
	}
}

func (s *Socket) RegisterRequestController(namespace string, ctrl any) error {
	t := reflect.TypeOf(ctrl)
	v := reflect.ValueOf(ctrl)
	if t == nil || t.Kind() != reflect.Pointer {
		return errors.New("opsocket: controller must be a non-nil pointer")
	}
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	ctxType := reflect.TypeOf((*Ctx)(nil))

	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		fullName := namespace + "." + m.Name
		if m.Type.NumIn() != 3 || m.Type.In(1) != ctxType {
			return fmt.Errorf("opsocket: %s must accept (*opsocket.Ctx, Request)", fullName)
		}
		if m.Type.NumOut() < 1 || m.Type.NumOut() > 2 || !m.Type.Out(m.Type.NumOut()-1).Implements(errorType) {
			return fmt.Errorf("opsocket: %s must return error or (Response, error)", fullName)
		}
		if _, exists := s.controllers[fullName]; exists {
			return fmt.Errorf("opsocket: duplicate operation %s", fullName)
		}
		reqType := m.Type.In(2)
		var resType reflect.Type
		if m.Type.NumOut() == 2 {
			resType = m.Type.Out(0)
		}

		s.controllers[fullName] = controllerType{
			name:     fullName,
			handler:  v.Method(i),
			reqType:  reqType,
			respType: resType,
		}
	}
	return nil
}

func (s *Socket) HandleRequest(
	ctx *Ctx,
	req protocol.RequestPayload,
) {
	started := time.Now()
	clientID := ctx.ClientID()
	s.debugf("→ operation=%s request=%s client=%s payload=%s", req.Type, req.ID, clientID, compactJSON(req.Data))
	c, ok := s.controllers[req.Type]
	if !ok {
		s.debugf("← operation=%s request=%s client=%s error=NOT_FOUND duration=%s", req.Type, req.ID, clientID, time.Since(started).Round(time.Millisecond))
		ctx.ReplyError(req.ID, req.Type, "NOT_FOUND", "controller not found")
		return
	}

	handler := func(callCtx *Ctx, raw json.RawMessage) (response any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				applog.Error(context.Background(), fmt.Sprintf("opsocket recovered operation panic operation=%s error=%v", req.Type, recovered))
				err = &Error{Code: "INTERNAL", Message: "operation failed"}
			}
		}()
		arg := reflect.New(c.reqType)
		if len(raw) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if decodeErr := decoder.Decode(arg.Interface()); decodeErr != nil {
				return nil, &Error{Code: "BAD_REQUEST", Message: decodeErr.Error()}
			}
		}
		results := c.handler.Call([]reflect.Value{reflect.ValueOf(callCtx), arg.Elem()})
		if !results[len(results)-1].IsNil() {
			return nil, results[len(results)-1].Interface().(error)
		}
		if c.respType != nil {
			return results[0].Interface(), nil
		}
		return nil, nil
	}
	for i := len(s.middleware) - 1; i >= 0; i-- {
		handler = s.middleware[i](req.Type, handler)
	}
	response, err := handler(ctx, req.Data)
	if err != nil {
		s.debugf("← operation=%s request=%s client=%s error=%v duration=%s", req.Type, req.ID, clientID, err, time.Since(started).Round(time.Millisecond))
		_ = ctx.ReplyOperationError(req.ID, req.Type, err)
		return
	}
	s.debugf("← operation=%s request=%s client=%s ok duration=%s", req.Type, req.ID, clientID, time.Since(started).Round(time.Millisecond))
	_ = ctx.ReplyOK(req.ID, req.Type, response)
}

func (s *baseSocket) debugf(format string, args ...any) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	applog.Debug(context.Background(), fmt.Sprintf("[opsocket] "+format, args...))
}

// lazyJSON defers compaction until a debug line is actually formatted.
type lazyJSON json.RawMessage

func (value lazyJSON) String() string { return compactJSON(json.RawMessage(value)) }

func compactJSON(value json.RawMessage) string {
	if len(value) == 0 {
		return "{}"
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return "<invalid-json>"
	}
	const max = 2048
	if compact.Len() > max {
		return compact.String()[:max] + "…"
	}
	return compact.String()
}

func (s *Socket) AttachToFiber(r fiber.Router, path string) {
	s.AttachToFiberWithOptions(r, path, AttachOptions{AllowAnonymous: true, OriginAllowed: sameOriginAllowed})
}

func (s *Socket) AttachToFiberWithOptions(r fiber.Router, path string, options AttachOptions) {
	if options.ReadLimit <= 0 {
		options.ReadLimit = 1 << 20
	}
	if options.OriginAllowed == nil {
		options.OriginAllowed = sameOriginAllowed
	}
	if options.AuthCheckInterval <= 0 {
		options.AuthCheckInterval = 30 * time.Second
	}
	r.Use(path, func(c *fiber.Ctx) error {
		if origin := c.Get("Origin"); origin != "" && !options.OriginAllowed(c, origin) {
			return fiber.NewError(fiber.StatusForbidden, "websocket origin rejected")
		}
		principal := Principal{}
		view := ""
		if c.Query("view") == ViewPublic {
			// Downgrade only: the audience display must never see private
			// state even when the browser carries an operator session, and a
			// stale cookie must not keep it from connecting. Sockets that
			// refuse anonymous principals refuse the public view too.
			if !options.AllowAnonymous && options.Authenticate != nil {
				return fiber.NewError(fiber.StatusUnauthorized, "websocket authentication required")
			}
			view = ViewPublic
		} else if options.Authenticate != nil {
			resolved, err := options.Authenticate(c)
			if err != nil && (!options.AllowAnonymous || options.RejectAuthenticationErrors) {
				return fiber.NewError(fiber.StatusUnauthorized, "websocket authentication failed")
			}
			if err == nil {
				principal = resolved
			}
		}
		c.Locals("opsocket.principal", principal)
		c.Locals("opsocket.view", view)
		return c.Next()
	})
	r.Get(path, websocket.New(func(c *websocket.Conn) {
		principal, _ := c.Locals("opsocket.principal").(Principal)
		view, _ := c.Locals("opsocket.view").(string)
		client := s.registerClient(c, principal, view)
		s.startWriter(client)
		defer func() {
			s.closeClient(client)
			// The writer must stop touching the connection before Fiber
			// recycles it.
			<-client.writerDone
		}()
		if principal.Authenticated() && options.ValidateAuthenticated != nil {
			go s.monitorAuthentication(client, options.ValidateAuthenticated, options.AuthCheckInterval)
		}
		c.SetReadLimit(options.ReadLimit)
		_ = c.SetReadDeadline(time.Now().Add(pongWait))
		c.SetPongHandler(func(string) error {
			return c.SetReadDeadline(time.Now().Add(pongWait))
		})

		for {
			_, rawMsg, err := c.ReadMessage()
			if err != nil {
				break
			}

			var msg protocol.Envelope
			{
				err := json.Unmarshal(rawMsg, &msg)
				if err != nil {
					continue
				}
			}

			switch msg.Operation {
			case protocol.OperationAuthentication:
				// Authentication is established during the HTTP upgrade.
			case protocol.OperationHeartbeat:
				var probe protocol.HeartbeatPayload
				_ = json.Unmarshal(msg.Data, &probe)
				_ = s.enqueue(client, heartbeatFrame(probe.ClientTime))
			case protocol.OperationRequest:
				var req protocol.RequestPayload
				if err := json.Unmarshal(msg.Data, &req); err != nil {
					continue
				}
				s.HandleRequest(&Ctx{Socket: s.baseSocket, Conn: c, client: client}, req)
			}
		}
	}, websocket.Config{
		RecoverHandler: s.recoverHandler(),
	}))
}

func (s *Socket) monitorAuthentication(client *Client, validate func(context.Context, Principal) bool, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-client.Ctx.Done():
			return
		case <-ticker.C:
			if !validate(client.Ctx, client.Principal) {
				s.closeClient(client)
				return
			}
		}
	}
}

func sameOriginAllowed(c *fiber.Ctx, rawOrigin string) bool {
	origin, err := url.Parse(rawOrigin)
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" {
		return false
	}
	return strings.EqualFold(origin.Host, c.Get("Host"))
}

// EmitEvent broadcasts an event to all connected clients.
func (s *Socket) EmitEvent(event string, data interface{}) {
	var dataRaw json.RawMessage
	if data != nil {
		b, _ := json.Marshal(data)
		dataRaw = json.RawMessage(b)
	}
	s.baseSocket.EmitEvent(event, dataRaw)
}

func (s *Socket) EmitEventToRoom(room, event string, data interface{}) {
	b, _ := json.Marshal(data)
	s.baseSocket.EmitEventToRoom(room, event, b)
}

// EmitEventToRooms sends one copy to each client in any of rooms. A nil data
// sends the event without a payload.
func (s *Socket) EmitEventToRooms(rooms []string, event string, data interface{}) {
	var raw json.RawMessage
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	s.baseSocket.EmitEventToRooms(rooms, event, raw)
}

func (s *Socket) RoomSize(room string) int {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	count := 0
	for _, client := range s.clients {
		if client.inRoom(room) {
			count++
		}
	}
	return count
}

func greetingEnvelope(client *Client, debug bool) protocol.Envelope {
	payload := protocol.GreetingPayload{
		ProtocolVersion: 1, ClientID: client.ID, ServerTime: time.Now().UnixMilli(),
		Authenticated: client.Principal.Authenticated(), Subject: client.Principal.Subject,
		Roles: append([]string(nil), client.Principal.Roles...),
		Debug: debug, View: client.View,
	}
	b, _ := json.Marshal(payload)
	return protocol.Envelope{Operation: protocol.OperationGreeting, Data: b}
}

func (s *Socket) recoverHandler() func(*websocket.Conn) {
	return func(c *websocket.Conn) {
		if err := recover(); err != nil {
			applog.Error(context.Background(), fmt.Sprintf("opsocket recovered websocket panic: %v", err))
		}
	}
}
