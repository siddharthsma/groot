package schema

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

type stubStore struct {
	upserted []Record
	byFull   map[string]Schema
	latest   map[string]Schema
}

func (s *stubStore) UpsertEventSchema(_ context.Context, record Record) error {
	s.upserted = append(s.upserted, record)
	if s.byFull == nil {
		s.byFull = make(map[string]Schema)
	}
	if s.latest == nil {
		s.latest = make(map[string]Schema)
	}
	schema := Schema{ID: record.ID, EventType: record.EventType, Version: record.Version, FullName: record.FullName, Source: record.Source, SourceKind: record.SourceKind, SchemaJSON: record.SchemaJSON}
	s.byFull[record.FullName] = schema
	if current, ok := s.latest[record.EventType]; !ok || record.Version > current.Version {
		s.latest[record.EventType] = schema
	}
	return nil
}

func (s *stubStore) ListEventSchemas(context.Context) ([]Schema, error) {
	var out []Schema
	for _, schema := range s.byFull {
		out = append(out, schema)
	}
	return out, nil
}

func (s *stubStore) GetEventSchema(_ context.Context, fullName string) (Schema, error) {
	schema, ok := s.byFull[fullName]
	if !ok {
		return Schema{}, sql.ErrNoRows
	}
	return schema, nil
}

func (s *stubStore) GetLatestEventSchema(_ context.Context, eventType string) (Schema, error) {
	schema, ok := s.latest[eventType]
	if !ok {
		return Schema{}, sql.ErrNoRows
	}
	return schema, nil
}

type stubMetrics struct {
	registered         int
	validationFailures int
	templateFailures   int
}

func (s *stubMetrics) IncSchemaRegistered()                       { s.registered++ }
func (s *stubMetrics) IncSchemaValidationFailures()               { s.validationFailures++ }
func (s *stubMetrics) IncSubscriptionTemplateValidationFailures() { s.templateFailures++ }

func TestRegisterBundles(t *testing.T) {
	store := &stubStore{}
	metrics := &stubMetrics{}
	svc := NewService(store, Config{ValidationMode: ValidationModeWarn, MaxPayloadBytes: 1024}, nil, metrics)
	if err := svc.RegisterBundles(context.Background(), CoreBundles()); err != nil {
		t.Fatalf("RegisterBundles() error = %v", err)
	}
	if len(store.upserted) == 0 {
		t.Fatal("expected schemas to be upserted")
	}
	if metrics.registered != len(store.upserted) {
		t.Fatalf("registered metrics = %d, want %d", metrics.registered, len(store.upserted))
	}
}

func TestValidateEventWarnVsReject(t *testing.T) {
	schemaJSON, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []string{"text"}, "additionalProperties": false})
	store := &stubStore{byFull: map[string]Schema{"example.event.v1": {FullName: "example.event.v1", EventType: "example.event", Version: 1, Source: "manual", SourceKind: "external", SchemaJSON: schemaJSON}}}
	warnMetrics := &stubMetrics{}
	warnSvc := NewService(store, Config{ValidationMode: ValidationModeWarn, MaxPayloadBytes: 1024}, nil, warnMetrics)
	_, err := warnSvc.ValidateEvent(context.Background(), "example.event.v1", "manual", "external", json.RawMessage(`{"bad":true}`))
	if err != nil {
		t.Fatalf("ValidateEvent(warn) error = %v", err)
	}
	if warnMetrics.validationFailures != 1 {
		t.Fatalf("validationFailures = %d", warnMetrics.validationFailures)
	}
	rejectSvc := NewService(store, Config{ValidationMode: ValidationModeReject, MaxPayloadBytes: 1024}, nil, &stubMetrics{})
	_, err = rejectSvc.ValidateEvent(context.Background(), "example.event.v1", "manual", "external", json.RawMessage(`{"bad":true}`))
	var rejectErr RejectError
	if !errors.As(err, &rejectErr) {
		t.Fatalf("ValidateEvent(reject) error = %v, want reject error", err)
	}
}

func TestValidateTemplatePaths(t *testing.T) {
	schemaJSON, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"output": map[string]any{"type": "object", "properties": map[string]any{"customer": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}}, "required": []string{"customer"}}}, "required": []string{"output"}})
	store := &stubStore{byFull: map[string]Schema{"llm.extract.completed.v1": {FullName: "llm.extract.completed.v1", SchemaJSON: schemaJSON}}}
	metrics := &stubMetrics{}
	svc := NewService(store, Config{ValidationMode: ValidationModeWarn, MaxPayloadBytes: 1024}, nil, metrics)
	if err := svc.ValidateTemplatePaths(context.Background(), "llm.extract.completed.v1", map[string]any{"text": "Hello {{payload.output.customer.name}}"}); err != nil {
		t.Fatalf("ValidateTemplatePaths(valid) error = %v", err)
	}
	err := svc.ValidateTemplatePaths(context.Background(), "llm.extract.completed.v1", map[string]any{"text": "Hello {{payload.output.customer.email}}"})
	var templateErr TemplatePathError
	if !errors.As(err, &templateErr) {
		t.Fatalf("ValidateTemplatePaths(invalid) error = %v", err)
	}
	if metrics.templateFailures != 1 {
		t.Fatalf("templateFailures = %d", metrics.templateFailures)
	}
}

func TestGetLatestSchema(t *testing.T) {
	store := &stubStore{latest: map[string]Schema{"llm.summarize.completed": {FullName: "llm.summarize.completed.v2", EventType: "llm.summarize.completed", Version: 2}}}
	svc := NewService(store, Config{ValidationMode: ValidationModeWarn, MaxPayloadBytes: 1024}, nil, &stubMetrics{})
	schema, err := svc.GetLatest(context.Background(), "llm.summarize.completed")
	if err != nil {
		t.Fatalf("GetLatest() error = %v", err)
	}
	if got, want := schema.FullName, "llm.summarize.completed.v2"; got != want {
		t.Fatalf("FullName = %q, want %q", got, want)
	}
}
