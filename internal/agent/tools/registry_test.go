package tools

import (
	"encoding/json"
	"testing"
)

func TestRegistryValidatesConnectorArgs(t *testing.T) {
	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}
	if err := registry.Validate("slack.create_thread_reply", json.RawMessage(`{"channel":"C123","text":"hello","thread_ts":"1710000000.1"}`)); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := registry.Validate("slack.create_thread_reply", json.RawMessage(`{"channel":"C123","text":"hello"}`)); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}
