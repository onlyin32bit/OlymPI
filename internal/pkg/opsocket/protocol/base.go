package protocol

import "encoding/json"

type OperationCode uint8

const (
	OperationGreeting OperationCode = iota
	OperationAuthentication
	OperationEvent
	OperationRequest
	OperationResponse
	// OperationHeartbeat flows both ways. The server sends one periodically so
	// browsers (which cannot observe WebSocket pings) can detect dead links and
	// track the server clock; a client may send {client_time} and receives an
	// echo with server_time to measure its clock offset with round-trip time.
	OperationHeartbeat
)

// type ProtocolVersion uint8

// const (
// 	ProtocolV1 ProtocolVersion = 1
// )

type Envelope struct {
	Operation OperationCode   `json:"o"`
	Data      json.RawMessage `json:"d"`
}

type RequestPayload struct {
	ID   string          `json:"requestId"`
	Type string          `json:"requestType"`
	Data json.RawMessage `json:"requestData,omitempty"`
}

type AuthenticatePayload struct {
	ID   string          `json:"requestId"`
	Type string          `json:"requestType"`
	Data json.RawMessage `json:"requestData,omitempty"`
}

type ResponsePayload struct {
	ID     string          `json:"requestId"`
	Type   string          `json:"requestType"`
	Status *ResponseStatus `json:"requestStatus"`
	Data   json.RawMessage `json:"responseData,omitempty"`
}

type EventMessage struct {
	Event string          `json:"eventType"`
	Data  json.RawMessage `json:"eventData,omitempty"`
}

type ResponseStatus struct {
	Ok        bool           `json:"ok"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
	Version   int64          `json:"version,omitempty"`
}

type GreetingPayload struct {
	ProtocolVersion int      `json:"protocol_version"`
	ClientID        string   `json:"client_id"`
	ServerTime      int64    `json:"server_time"`
	Authenticated   bool     `json:"authenticated"`
	Subject         string   `json:"subject,omitempty"`
	Roles           []string `json:"roles,omitempty"`
	Debug           bool     `json:"debug,omitempty"`
	View            string   `json:"view,omitempty"`
}

type HeartbeatPayload struct {
	ServerTime int64 `json:"server_time,omitempty"`
	ClientTime int64 `json:"client_time,omitempty"`
}
