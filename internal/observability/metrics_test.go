package observability

import (
	"strings"
	"testing"
)

func TestLLMMetricsLabels(t *testing.T) {
	metrics := NewMetrics()
	metrics.IncLLMRequests("openai", "generate")
	metrics.IncLLMFailures("anthropic")
	metrics.ObserveLLMLatency("openai", "generate", 0.2)
	metrics.IncResendEmailsSent()
	metrics.IncSlackEventsReceived()
	metrics.IncSlackThreadReplies()
	metrics.IncLLMClassifications()
	metrics.IncLLMExtractions()
	metrics.IncResultEventsEmitted("llm", "summarize", "succeeded")
	metrics.IncResultEventEmitFailures()
	metrics.IncGraphRequests()
	metrics.AddGraphNodes(7)
	metrics.AddGraphEdges(9)
	metrics.IncGraphLimitExceeded()
	metrics.SetEditionInfo("community", "single")
	metrics.SetLicenseInfo("community", true)

	output := metrics.Prometheus()
	if !strings.Contains(output, `groot_llm_requests_total{provider="openai",operation="generate"} 1`) {
		t.Fatalf("missing llm requests metric: %s", output)
	}
	if !strings.Contains(output, `groot_llm_failures_total{provider="anthropic"} 1`) {
		t.Fatalf("missing llm failures metric: %s", output)
	}
	if !strings.Contains(output, `groot_llm_latency_seconds_bucket{provider="openai",operation="generate",le="0.25"} 1`) {
		t.Fatalf("missing llm latency metric: %s", output)
	}
	if !strings.Contains(output, `groot_resend_emails_sent_total 1`) {
		t.Fatalf("missing resend emails metric: %s", output)
	}
	if !strings.Contains(output, `groot_slack_events_received_total 1`) {
		t.Fatalf("missing slack events metric: %s", output)
	}
	if !strings.Contains(output, `groot_slack_thread_replies_total 1`) {
		t.Fatalf("missing slack thread reply metric: %s", output)
	}
	if !strings.Contains(output, `groot_llm_classifications_total 1`) {
		t.Fatalf("missing llm classifications metric: %s", output)
	}
	if !strings.Contains(output, `groot_llm_extractions_total 1`) {
		t.Fatalf("missing llm extractions metric: %s", output)
	}
	if !strings.Contains(output, `groot_result_events_emitted_total{connector="llm",operation="summarize",status="succeeded"} 1`) {
		t.Fatalf("missing result event metric: %s", output)
	}
	if !strings.Contains(output, `groot_result_event_emit_failures_total 1`) {
		t.Fatalf("missing result event failure metric: %s", output)
	}
	if !strings.Contains(output, `groot_graph_requests_total 1`) {
		t.Fatalf("missing graph requests metric: %s", output)
	}
	if !strings.Contains(output, `groot_graph_nodes_total 7`) {
		t.Fatalf("missing graph nodes metric: %s", output)
	}
	if !strings.Contains(output, `groot_graph_edges_total 9`) {
		t.Fatalf("missing graph edges metric: %s", output)
	}
	if !strings.Contains(output, `groot_graph_limit_exceeded_total 1`) {
		t.Fatalf("missing graph limit metric: %s", output)
	}
	if !strings.Contains(output, `groot_edition_info{edition="community",tenancy_mode="single"} 1`) {
		t.Fatalf("missing edition info metric: %s", output)
	}
	if !strings.Contains(output, `groot_license_info{edition="community",license_present="true"} 1`) {
		t.Fatalf("missing license info metric: %s", output)
	}
}
