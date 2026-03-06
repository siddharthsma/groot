package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"groot/internal/config"
	"groot/internal/connectors/outbound"
	"groot/internal/stream"
)

func TestGenerateOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/chat/completions"; got != want {
			t.Fatalf("Path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"summary"}}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`))
	}))
	defer server.Close()

	connector := New(config.LLMConfig{
		OpenAIAPIKey:     "openai-secret",
		OpenAIAPIBaseURL: server.URL,
		DefaultProvider:  "openai",
		TimeoutSeconds:   30,
	}, server.Client())

	result, err := connector.Execute(context.Background(), OperationGenerate,
		json.RawMessage(`{"default_provider":"openai","providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
		json.RawMessage(`{"prompt":"Summarize {{payload.text}}","model":"gpt-4o-mini"}`),
		stream.Event{},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.Text, "summary"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if got, want := result.Usage.TotalTokens, 14; got != want {
		t.Fatalf("TotalTokens = %d, want %d", got, want)
	}
}

func TestSummarizeAnthropicRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	connector := New(config.LLMConfig{
		AnthropicAPIKey:     "anthropic-secret",
		AnthropicAPIBaseURL: server.URL,
		DefaultProvider:     "openai",
		TimeoutSeconds:      30,
	}, server.Client())

	_, err := connector.Execute(context.Background(), OperationSummarize,
		json.RawMessage(`{"default_provider":"anthropic","providers":{"anthropic":{"api_key":"env:ANTHROPIC_API_KEY"}}}`),
		json.RawMessage(`{"text":"Long text","provider":"anthropic"}`),
		stream.Event{},
	)
	var retryable outbound.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("Execute() error = %v, want retryable error", err)
	}
}

func TestClassifyUsesConfiguredDefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, want := body["model"], "gpt-4o-mini"; got != want {
			t.Fatalf("model = %v, want %v", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"support"}}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`))
	}))
	defer server.Close()

	connector := New(config.LLMConfig{
		OpenAIAPIKey:         "openai-secret",
		OpenAIAPIBaseURL:     server.URL,
		DefaultProvider:      "openai",
		DefaultClassifyModel: "gpt-4o-mini",
		TimeoutSeconds:       30,
	}, server.Client())

	result, err := connector.Execute(context.Background(), OperationClassify,
		json.RawMessage(`{"default_provider":"openai","providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
		json.RawMessage(`{"text":"Customer wants a refund","labels":["sales","support","spam"]}`),
		stream.Event{},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := string(result.Output), `{"label":"support"}`; got != want {
		t.Fatalf("Output = %s, want %s", got, want)
	}
}

func TestExtractReturnsStructuredJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"customer\":{\"name\":\"Ada\"},\"urgent\":true}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	connector := New(config.LLMConfig{
		OpenAIAPIKey:        "openai-secret",
		OpenAIAPIBaseURL:    server.URL,
		DefaultProvider:     "openai",
		DefaultExtractModel: "gpt-4o-mini",
		TimeoutSeconds:      30,
	}, server.Client())

	result, err := connector.Execute(context.Background(), OperationExtract,
		json.RawMessage(`{"default_provider":"openai","providers":{"openai":{"api_key":"env:OPENAI_API_KEY"}}}`),
		json.RawMessage(`{"text":"Ada needs help","schema":{"type":"object"}}`),
		stream.Event{},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := string(result.Output), `{"customer":{"name":"Ada"},"urgent":true}`; got != want {
		t.Fatalf("Output = %s, want %s", got, want)
	}
}
