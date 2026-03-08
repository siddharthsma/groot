package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"groot/internal/schema"
)

type stubSchemas struct {
	getFn func(context.Context, string) (schema.Schema, error)
}

func (s stubSchemas) Get(ctx context.Context, fullName string) (schema.Schema, error) {
	return s.getFn(ctx, fullName)
}

func TestListIncludesRegisteredProviders(t *testing.T) {
	svc := NewService(nil)
	providers, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected providers")
	}
	if providers[0].Name == "" {
		t.Fatal("expected provider name")
	}
}

func TestGetMatchesSpec(t *testing.T) {
	svc := NewService(nil)
	detail, err := svc.Get(context.Background(), "slack")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if detail.Name != "slack" {
		t.Fatalf("Name = %q", detail.Name)
	}
	if len(detail.Config.Fields) == 0 {
		t.Fatal("expected config fields")
	}
	if len(detail.Operations) != 2 {
		t.Fatalf("operation count = %d", len(detail.Operations))
	}
}

func TestValidateDetectsSchemaMismatch(t *testing.T) {
	svc := NewService(stubSchemas{
		getFn: func(context.Context, string) (schema.Schema, error) {
			body, _ := json.Marshal(map[string]any{"type": "object"})
			return schema.Schema{
				FullName:   "slack.message.created.v1",
				EventType:  "slack.message.created",
				Version:    1,
				Source:     "slack",
				SourceKind: "external",
				SchemaJSON: body,
			}, nil
		},
	})
	if err := svc.Validate(context.Background()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateMissingSchemaFails(t *testing.T) {
	svc := NewService(stubSchemas{
		getFn: func(context.Context, string) (schema.Schema, error) {
			return schema.Schema{}, sql.ErrNoRows
		},
	})
	if err := svc.Validate(context.Background()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSameJSONIgnoresRequiredArrayOrder(t *testing.T) {
	left := json.RawMessage(`{"type":"object","required":["b","a"],"properties":{"a":{"type":"string"},"b":{"type":"string"}}}`)
	right := json.RawMessage(`{"properties":{"b":{"type":"string"},"a":{"type":"string"}},"required":["a","b"],"type":"object"}`)
	match, err := sameJSON(left, right)
	if err != nil {
		t.Fatalf("sameJSON() error = %v", err)
	}
	if !match {
		t.Fatal("expected schema JSON to match")
	}
}
