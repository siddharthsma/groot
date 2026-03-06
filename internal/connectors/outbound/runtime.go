package outbound

import (
	"context"
	"encoding/json"
	"errors"

	"groot/internal/stream"
)

type Result struct {
	ExternalID string
	StatusCode int
}

type RetryableError struct {
	Err        error
	StatusCode int
}

func (e RetryableError) Error() string { return e.Err.Error() }
func (e RetryableError) Unwrap() error { return e.Err }

type PermanentError struct {
	Err        error
	StatusCode int
}

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

var ErrUnsupportedOperation = errors.New("unsupported connector operation")

type Connector interface {
	Name() string
	Execute(context.Context, string, json.RawMessage, json.RawMessage, stream.Event) (Result, error)
}
