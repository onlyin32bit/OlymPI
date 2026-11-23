package opsocket

import (
	"context"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

type ConnCtxKeyType string

const ConnCtxKey ConnCtxKeyType = "opsocket.connection"

type Principal struct {
	Subject   string            `json:"subject"`
	Roles     []string          `json:"roles,omitempty"`
	Claims    map[string]string `json:"claims,omitempty"`
	SessionID string            `json:"-"`
}

func (p Principal) Authenticated() bool { return p.Subject != "" }

func (p Principal) HasRole(role string) bool {
	for _, candidate := range p.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

type Client struct {
	ID        string
	IsAlive   bool
	Ctx       context.Context
	Cancel    context.CancelFunc
	Principal Principal
	// View is the requested presentation ("public" for audience displays).
	View string
	conn *websocket.Conn
	// lifecycle guards closed and the send queue. Close holds it while closing
	// the connection, so the handler's deferred close cannot return (and let
	// Fiber recycle its pooled wrapper) while another goroutine is closing it.
	lifecycle sync.Mutex
	closed    bool
	// send is the bounded outbound queue drained by this client's writer
	// goroutine. Nil for clients without a writer (never connected).
	send       chan []byte
	writerDone chan struct{}
	roomsMu    sync.RWMutex
	rooms      map[string]struct{}
}

func (c *Client) join(room string) {
	if room == "" {
		return
	}
	c.roomsMu.Lock()
	c.rooms[room] = struct{}{}
	c.roomsMu.Unlock()
}

func (c *Client) leave(room string) {
	c.roomsMu.Lock()
	delete(c.rooms, room)
	c.roomsMu.Unlock()
}

func (c *Client) inRoom(room string) bool {
	c.roomsMu.RLock()
	_, ok := c.rooms[room]
	c.roomsMu.RUnlock()
	return ok
}
