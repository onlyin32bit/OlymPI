package opsocket

import (
	"errors"
	"fmt"
)

var (
	ErrControllerNotFound = errors.New("controller not found")
	ErrBadRequest         = errors.New("bad request")
)

// Error is safe to serialize to a socket client. Internal errors should be
// logged by the application and returned as CodeInternal without Details.
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
	Version   int64          `json:"version,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NewError(code, message string) *Error { return &Error{Code: code, Message: message} }

func WrapError(code, message string, err error) *Error {
	if err == nil {
		return NewError(code, message)
	}
	return &Error{Code: code, Message: fmt.Sprintf("%s: %v", message, err)}
}
