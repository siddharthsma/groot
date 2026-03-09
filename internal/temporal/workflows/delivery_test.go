package workflows

import (
	"encoding/json"
	"testing"

	"groot/internal/connectors/outbound"
	"groot/internal/integrations/llm"
	"groot/internal/temporal/activities"
)

func TestConnectorOutputAugmentsLLMGenerateMetadata(t *testing.T) {
	output, err := connectorOutput(llm.IntegrationName, llm.OperationGenerate, activities.ConnectionResult{
		Output:      []byte(`{"text":"summary"}`),
		Integration: "openai",
		Model:       "gpt-4o-mini",
		Usage: outbound.Usage{
			PromptTokens:     3,
			CompletionTokens: 5,
			TotalTokens:      8,
		},
	})
	if err != nil {
		t.Fatalf("connectorOutput() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := decoded["text"]; got != "summary" {
		t.Fatalf("text = %v", got)
	}
	if got := decoded["integration"]; got != "openai" {
		t.Fatalf("integration = %v", got)
	}
	if got := decoded["model"]; got != "gpt-4o-mini" {
		t.Fatalf("model = %v", got)
	}
	usage, ok := decoded["usage"].(map[string]any)
	if !ok {
		t.Fatalf("usage type = %T", decoded["usage"])
	}
	if got := usage["total_tokens"]; got != float64(8) {
		t.Fatalf("usage.total_tokens = %v", got)
	}
}
