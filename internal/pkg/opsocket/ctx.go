package opsocket

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gofiber/contrib/websocket"
	"github.com/onlyin32bit/4t/internal/pkg/opsocket/protocol"
)

type Ctx struct {
	Socket *baseSocket
	Data   json.RawMessage
	Conn   *websocket.Conn
	Action string
	client *Client
}

func (c *Ctx) Unmarshal(v any) error {
	if c.Data == nil {
		return nil
	}
	return json.Unmarshal(c.Data, v)
}

func (c *Ctx) clientState() *Client {
	if c.client != nil {
		return c.client
	}
	if c.Socket == nil || c.Conn == nil {
		return nil
	}
	c.Socket.clientsMutex.RLock()
	defer c.Socket.clientsMutex.RUnlock()
	return c.Socket.clients[c.Conn]
}

func (c *Ctx) Context() context.Context {
	client := c.clientState()
	if client == nil || client.Ctx == nil {
		return context.Background()
	}
	return client.Ctx
}

func (c *Ctx) ClientID() string {
	client := c.clientState()
	if client == nil {
		return ""
	}
	return client.ID
}

func (c *Ctx) Principal() Principal {
	client := c.clientState()
	if client == nil {
		return Principal{}
	}
	return client.Principal
}

func (c *Ctx) IsAuthenticated() bool { return c.Principal().Authenticated() }

func (c *Ctx) RequireRole(role string) error {
	if !c.Principal().HasRole(role) {
		return &Error{Code: "FORBIDDEN", Message: "insufficient permissions"}
	}
	return nil
}

func (c *Ctx) Join(room string) {
	client := c.clientState()
	if client != nil {
		client.join(room)
		c.Socket.debugf("joined client=%s room=%s", client.ID, room)
	}
}

func (c *Ctx) Leave(room string) {
	client := c.clientState()
	if client != nil {
		client.leave(room)
		c.Socket.debugf("left client=%s room=%s", client.ID, room)
	}
}

func (c *Ctx) EmitToThisConn(event string, data interface{}) error {
	if c.Socket == nil || c.Conn == nil {
		return nil
	}
	return c.Socket.emitTo(c.clientState(), event, data)
}

func (c *Ctx) ReplyOK(requestID, requestType string, data interface{}) error {
	return c.Reply(requestID, requestType, &protocol.ResponseStatus{Ok: true}, data)
}

func (c *Ctx) ReplyError(requestID, requestType, code, message string) error {
	return c.Reply(requestID, requestType, &protocol.ResponseStatus{Code: code, Message: message}, nil)
}

func (c *Ctx) ReplyOperationError(requestID, requestType string, err error) error {
	var operationError *Error
	if !errors.As(err, &operationError) {
		operationError = &Error{Code: "INTERNAL", Message: "operation failed"}
	}
	return c.Reply(requestID, requestType, &protocol.ResponseStatus{
		Code: operationError.Code, Message: operationError.Message, Details: operationError.Details,
		Retryable: operationError.Retryable, Version: operationError.Version,
	}, nil)
}

func (c *Ctx) Reply(requestID string, requestType string, statusPayload *protocol.ResponseStatus, data interface{}) error {
	if c.Socket == nil || c.Conn == nil {
		return nil
	}

	var dataRaw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		dataRaw = json.RawMessage(b)
	}

	var resp protocol.ResponsePayload
	if statusPayload != nil {
		resp = protocol.ResponsePayload{ID: requestID, Type: requestType, Status: statusPayload, Data: dataRaw}
	} else {
		resp = protocol.ResponsePayload{ID: requestID, Type: requestType, Data: dataRaw}
	}

	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	env := protocol.Envelope{Operation: protocol.OperationResponse, Data: json.RawMessage(b)}
	return c.Socket.writeJSON(c.clientState(), env)
}
