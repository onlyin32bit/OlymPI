package opsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
	applog "github.com/onlyin32bit/4t/internal/shared/logger"
)

// Fan-out never writes to a connection directly. Each client owns a bounded
// queue drained by its own writer goroutine, so a slow or dead client cannot
// stall other clients or the publisher (which may hold the game lock). A
// client whose queue overflows, or whose write exceeds writeWait, is
// disconnected; it reconnects and resynchronizes from a fresh snapshot.

// encodeEvent marshals once per event; every recipient shares the frame.
func encodeEvent(event string, data json.RawMessage) []byte {
	em, _ := json.Marshal(protocol.EventMessage{Event: event, Data: data})
	frame := make([]byte, 0, len(em)+16)
	frame = fmt.Appendf(frame, `{"o":%d,"d":`, protocol.OperationEvent)
	frame = append(frame, em...)
	return append(frame, '}')
}

func (s *baseSocket) snapshotClients(matches func(*Client) bool) []*Client {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	conns := make([]*Client, 0, len(s.clients))
	for _, client := range s.clients {
		if matches == nil || matches(client) {
			conns = append(conns, client)
		}
	}
	return conns
}

func (s *baseSocket) fanOut(frame []byte, targets []*Client) {
	for _, client := range targets {
		_ = s.enqueue(client, frame)
	}
}

func (s *baseSocket) EmitEvent(event string, data json.RawMessage) {
	targets := s.snapshotClients(nil)
	s.debugf("⇢ event=%s room=* recipients=%d payload=%s", event, len(targets), lazyJSON(data))
	s.fanOut(encodeEvent(event, data), targets)
}

func (s *baseSocket) EmitEventToRoom(room, event string, data json.RawMessage) {
	s.EmitEventToRooms([]string{room}, event, data)
}

// EmitEventToRooms delivers one copy to every client in any of rooms.
func (s *baseSocket) EmitEventToRooms(rooms []string, event string, data json.RawMessage) {
	targets := s.snapshotClients(func(client *Client) bool {
		for _, room := range rooms {
			if client.inRoom(room) {
				return true
			}
		}
		return false
	})
	s.debugf("⇢ event=%s rooms=%v recipients=%d payload=%s", event, rooms, len(targets), lazyJSON(data))
	s.fanOut(encodeEvent(event, data), targets)
}

func (s *baseSocket) emitTo(client *Client, event string, data interface{}) error {
	var dataRaw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		dataRaw = json.RawMessage(b)
	}
	clientID := "unknown"
	if client != nil {
		clientID = client.ID
	}
	s.debugf("⇢ event=%s client=%s payload=%s", event, clientID, lazyJSON(dataRaw))
	return s.enqueue(client, encodeEvent(event, dataRaw))
}

// writeJSON queues v for the client's writer, preserving per-client order.
func (s *baseSocket) writeJSON(client *Client, v interface{}) error {
	frame, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.enqueue(client, frame)
}

// enqueue never blocks. Overflow disconnects only the slow client.
func (s *baseSocket) enqueue(client *Client, frame []byte) error {
	if client == nil {
		return net.ErrClosed
	}
	client.lifecycle.Lock()
	if client.closed || client.send == nil {
		client.lifecycle.Unlock()
		return net.ErrClosed
	}
	select {
	case client.send <- frame:
		client.lifecycle.Unlock()
		return nil
	default:
	}
	client.lifecycle.Unlock()
	applog.Warn(context.Background(), "opsocket client send queue full; disconnecting slow client", "client", client.ID, "queue", cap(client.send))
	s.closeClient(client)
	return errSendQueueFull
}

var errSendQueueFull = &Error{Code: "SLOW_CLIENT", Message: "send queue full", Retryable: true}

func (s *baseSocket) startWriter(client *Client) {
	client.writerDone = make(chan struct{})
	go s.writeLoop(client)
}

// writeLoop is the only goroutine writing data frames to the connection.
func (s *baseSocket) writeLoop(client *Client) {
	defer close(client.writerDone)
	ticker := time.NewTicker(s.pingInterval())
	defer ticker.Stop()
	conn := client.conn
	for {
		select {
		case <-client.Ctx.Done():
			return
		case frame := <-client.send:
			_ = conn.SetWriteDeadline(time.Now().Add(s.writeTimeout()))
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				if client.Ctx.Err() == nil {
					applog.Warn(context.Background(), "opsocket write failed; closing connection", "client", client.ID, "error", err)
				}
				s.closeClient(client)
				return
			}
		case <-ticker.C:
			// Protocol ping keeps the server read deadline alive; the
			// application heartbeat lets browsers detect a dead link and
			// track the server clock (browsers cannot see pings).
			deadline := time.Now().Add(s.writeTimeout())
			if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
				s.closeClient(client)
				return
			}
			_ = conn.SetWriteDeadline(deadline)
			if err := conn.WriteMessage(websocket.TextMessage, heartbeatFrame(0)); err != nil {
				s.closeClient(client)
				return
			}
		}
	}
}

func heartbeatFrame(clientTime int64) []byte {
	payload, _ := json.Marshal(protocol.HeartbeatPayload{ServerTime: time.Now().UnixMilli(), ClientTime: clientTime})
	frame, _ := json.Marshal(protocol.Envelope{Operation: protocol.OperationHeartbeat, Data: payload})
	return frame
}
