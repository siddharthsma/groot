//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"groot/tests/helpers"
)

func TestScenarioEmailTriage(t *testing.T) {
	h := helpers.NewHarness(t, helpers.HarnessOptions{})
	h.Mocks.QueueLLMResponses("support")

	tenantID, legacyKey := h.CreateTenant("phase20-email")
	llmConnectorID := createGlobalLLMConnector(t, h)

	systemHeaders := make(http.Header)
	systemHeaders.Set("Authorization", "Bearer system-secret")
	resp, body := h.JSONRequest(http.MethodPost, "/system/resend/bootstrap", systemHeaders, nil)
	mustStatus(t, resp, body, http.StatusOK)

	resp, body = h.JSONRequest(http.MethodPost, "/connectors/resend/enable", bearerHeader(legacyKey), nil)
	mustStatus(t, resp, body, http.StatusOK)
	address := mustString(t, decodeBody(t, body)["address"])

	resp, body = h.JSONRequest(http.MethodPost, "/connector-instances", bearerHeader(legacyKey), map[string]any{
		"connector_name": "slack",
		"config": map[string]any{
			"bot_token":       "xoxb-phase20",
			"default_channel": "C123",
		},
	})
	mustStatus(t, resp, body, http.StatusOK)
	slackConnectorID := mustString(t, decodeBody(t, body)["id"])

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type":      "connector",
		"connector_instance_id": llmConnectorID,
		"operation":             "classify",
		"operation_params": map[string]any{
			"text":   "{{payload.subject}} {{payload.text}}",
			"labels": []string{"support", "ignore"},
		},
		"event_type":         "resend.email.received.v1",
		"event_source":       "resend",
		"emit_success_event": true,
	})
	mustStatus(t, resp, body, http.StatusCreated)

	resp, body = h.JSONRequest(http.MethodPost, "/subscriptions", bearerHeader(legacyKey), map[string]any{
		"destination_type":      "connector",
		"connector_instance_id": slackConnectorID,
		"operation":             "post_message",
		"operation_params": map[string]any{
			"text": "triaged {{payload.output.label}}",
		},
		"filter": map[string]any{
			"path":  "payload.output.label",
			"op":    "==",
			"value": "support",
		},
		"event_type":         "llm.classify.completed.v1",
		"event_source":       "llm",
		"emit_success_event": true,
	})
	mustStatus(t, resp, body, http.StatusCreated)

	webhookPayload := map[string]any{
		"type":    "email.received",
		"to":      []string{address},
		"subject": "Need help",
		"text":    "customer escalation",
	}
	rawWebhook, _ := json.Marshal(webhookPayload)
	headers, err := h.Mocks.SignResend(rawWebhook)
	if err != nil {
		t.Fatalf("sign resend webhook: %v", err)
	}
	resp, body = h.Request(http.MethodPost, "/webhooks/resend", headers, bytesReader(rawWebhook))
	mustStatus(t, resp, body, http.StatusOK)

	rootEvent := eventByType(t, h, tenantID, "resend.email.received.v1")
	classifiedEvent := eventByType(t, h, tenantID, "llm.classify.completed.v1")
	_ = deliveryByEventStatus(t, h, tenantID, mustString(t, rootEvent["event_id"]), "succeeded")
	_ = deliveryByEventStatus(t, h, tenantID, mustString(t, classifiedEvent["event_id"]), "succeeded")

	waitForHTTPRequest(t, 15*time.Second, func() bool {
		return len(h.Mocks.SlackMessages()) >= 1
	})
	msg := h.Mocks.SlackMessages()[0]
	if got := mustString(t, msg["text"]); got != "triaged support" {
		t.Fatalf("slack text = %q, want %q", got, "triaged support")
	}

	waitForAuditEvent(t, h.DB, "subscription.create", 10*time.Second)
	h.AssertNoSecrets(legacyKey, h.AdminKey, "phase20-openai-key", "phase20-resend-api-key")
}
