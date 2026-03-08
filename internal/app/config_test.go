package app

import "testing"

func TestDeriveInternalToolEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		httpAddr string
		want     string
	}{
		{
			name:     "host and port",
			httpAddr: "127.0.0.1:8081",
			want:     "http://127.0.0.1:8081/internal/agent-runtime/tool-calls",
		},
		{
			name:     "wildcard host",
			httpAddr: "0.0.0.0:8081",
			want:     "http://127.0.0.1:8081/internal/agent-runtime/tool-calls",
		},
		{
			name:     "port only",
			httpAddr: ":9090",
			want:     "http://127.0.0.1:9090/internal/agent-runtime/tool-calls",
		},
		{
			name:     "full url",
			httpAddr: "https://example.com/api",
			want:     "https://example.com/api/internal/agent-runtime/tool-calls",
		},
		{
			name:     "empty",
			httpAddr: "",
			want:     "http://127.0.0.1:8081/internal/agent-runtime/tool-calls",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveInternalToolEndpoint(tt.httpAddr); got != tt.want {
				t.Fatalf("deriveInternalToolEndpoint(%q) = %q, want %q", tt.httpAddr, got, tt.want)
			}
		})
	}
}
