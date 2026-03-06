package subscriptionfilter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/schemas"
)

type stubSchemaLookup struct {
	getFn func(context.Context, string) (schemas.Schema, error)
}

func (s stubSchemaLookup) Get(ctx context.Context, fullName string) (schemas.Schema, error) {
	return s.getFn(ctx, fullName)
}

func TestEvaluateNestedFilter(t *testing.T) {
	filter := json.RawMessage(`{"all":[{"path":"payload.currency","op":"==","value":"usd"},{"any":[{"path":"payload.amount","op":">=","value":100},{"path":"payload.vip","op":"==","value":true}]}]}`)
	payload := json.RawMessage(`{"currency":"usd","amount":120,"vip":false}`)
	matched, err := Evaluate(filter, payload)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !matched {
		t.Fatal("Evaluate() = false, want true")
	}
}

func TestValidateAgainstSchemaRejectsInvalidPathAndOp(t *testing.T) {
	service := NewService(stubSchemaLookup{
		getFn: func(context.Context, string) (schemas.Schema, error) {
			return schemas.Schema{
				ID:         uuid.New(),
				FullName:   "payments.created.v1",
				SchemaJSON: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"currency":{"type":"string"},"amount":{"type":"number"}}}`),
			}, nil
		},
	})
	_, _, err := service.Validate(context.Background(), "payments.created.v1", json.RawMessage(`{"all":[{"path":"payload.missing","op":"==","value":"usd"},{"path":"payload.currency","op":">=","value":100}]}`))
	var validationErr ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Validate() error = %v, want validation error", err)
	}
	if got, want := len(validationErr.InvalidPaths), 1; got != want {
		t.Fatalf("len(InvalidPaths) = %d, want %d", got, want)
	}
	if got, want := len(validationErr.InvalidOps), 1; got != want {
		t.Fatalf("len(InvalidOps) = %d, want %d", got, want)
	}
}

func TestValidateAllowsMissingSchemaWithWarning(t *testing.T) {
	service := NewService(stubSchemaLookup{
		getFn: func(context.Context, string) (schemas.Schema, error) {
			return schemas.Schema{}, sql.ErrNoRows
		},
	})
	normalized, warnings, err := service.Validate(context.Background(), "example.event.v1", json.RawMessage(`{"path":"payload.currency","op":"==","value":"usd"}`))
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(normalized) == 0 {
		t.Fatal("normalized filter is empty")
	}
	if got, want := len(warnings), 1; got != want {
		t.Fatalf("len(warnings) = %d, want %d", got, want)
	}
	if got, want := warnings[0], WarningSchemaMissing; got != want {
		t.Fatalf("warning = %q, want %q", got, want)
	}
}
