package outbound

import (
	"context"
	"encoding/json"
	"errors"

	"groot/internal/event"
)

type Result struct {
	ExternalID  string
	StatusCode  int
	Channel     string
	Text        string
	Output      json.RawMessage
	Integration string
	Model       string
	Usage       Usage
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type RetryableError struct {
	Err         error
	StatusCode  int
	Integration string
	Model       string
}

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

type PermanentError struct {
	Err         error
	StatusCode  int
	Integration string
	Model       string
}

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

var ErrUnsupportedOperation = errors.New("unsupported connection operation")

type Connection interface {
	Name() string
	Execute(context.Context, string, json.RawMessage, json.RawMessage, event.Event) (Result, error)
}
