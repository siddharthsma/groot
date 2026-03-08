package observability

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	eventsReceived                         atomic.Uint64
	eventsPublished                        atomic.Uint64
	eventsRecorded                         atomic.Uint64
	routerEventsConsumed                   atomic.Uint64
	routerMatches                          atomic.Uint64
	deliveryStarted                        atomic.Uint64
	deliverySucceeded                      atomic.Uint64
	deliveryFailed                         atomic.Uint64
	deliveryDeadLetter                     atomic.Uint64
	functionInvocations                    atomic.Uint64
	functionInvocationFailures             atomic.Uint64
	resendWebhooksReceived                 atomic.Uint64
	resendWebhooksVerified                 atomic.Uint64
	resendVerificationFailed               atomic.Uint64
	resendUnroutable                       atomic.Uint64
	resendEventsPublished                  atomic.Uint64
	resendEmailsSent                       atomic.Uint64
	slackEventsReceived                    atomic.Uint64
	slackThreadReplies                     atomic.Uint64
	stripeWebhooks                         atomic.Uint64
	stripeUnroutable                       atomic.Uint64
	notionActions                          atomic.Uint64
	notionActionFailures                   atomic.Uint64
	llmClassifications                     atomic.Uint64
	llmExtractions                         atomic.Uint64
	llmLatencySum                          map[string]float64
	llmLatencyCount                        map[string]uint64
	replayRequests                         atomic.Uint64
	replayJobsCreated                      atomic.Uint64
	deliveryRetries                        atomic.Uint64
	agentRuns                              atomic.Uint64
	agentSteps                             atomic.Uint64
	agentToolCalls                         atomic.Uint64
	subscriptionFilterEvaluations          atomic.Uint64
	subscriptionFilterMatches              atomic.Uint64
	subscriptionFilterRejections           atomic.Uint64
	resultEventEmitFailures                atomic.Uint64
	schemaValidationFailures               atomic.Uint64
	schemaRegistered                       atomic.Uint64
	subscriptionTemplateValidationFailures atomic.Uint64
	graphRequests                          atomic.Uint64
	graphNodesTotal                        atomic.Uint64
	graphEdgesTotal                        atomic.Uint64
	graphLimitExceeded                     atomic.Uint64
	editionInfo                            atomic.Value
	licenseInfo                            atomic.Value

	mu                          sync.Mutex
	connectorDeliveries         map[string]uint64
	connectorDeliveryFailures   map[string]uint64
	connectorDeliveryDeadLetter map[string]uint64
	inboundRoutes               map[string]uint64
	inboundUnroutable           map[string]uint64
	globalConnectorDeliveries   map[string]uint64
	llmRequests                 map[string]uint64
	llmFailures                 map[string]uint64
	llmLatencyBuckets           map[string][]uint64
	resultEventsEmitted         map[string]uint64
}

type EditionInfo struct {
	Edition     string
	TenancyMode string
}

type LicenseInfo struct {
	Edition        string
	LicensePresent bool
}

var llmLatencyBounds = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

func NewMetrics() *Metrics {
	return &Metrics{
		connectorDeliveries:         make(map[string]uint64),
		connectorDeliveryFailures:   make(map[string]uint64),
		connectorDeliveryDeadLetter: make(map[string]uint64),
		inboundRoutes:               make(map[string]uint64),
		inboundUnroutable:           make(map[string]uint64),
		globalConnectorDeliveries:   make(map[string]uint64),
		llmRequests:                 make(map[string]uint64),
		llmFailures:                 make(map[string]uint64),
		llmLatencyBuckets:           make(map[string][]uint64),
		llmLatencySum:               make(map[string]float64),
		llmLatencyCount:             make(map[string]uint64),
		resultEventsEmitted:         make(map[string]uint64),
	}
}

func (m *Metrics) IncEventsReceived()             { m.eventsReceived.Add(1) }
func (m *Metrics) IncEventsPublished()            { m.eventsPublished.Add(1) }
func (m *Metrics) IncEventsRecorded()             { m.eventsRecorded.Add(1) }
func (m *Metrics) IncRouterEventsConsumed()       { m.routerEventsConsumed.Add(1) }
func (m *Metrics) IncRouterMatches()              { m.routerMatches.Add(1) }
func (m *Metrics) IncDeliveryStarted()            { m.deliveryStarted.Add(1) }
func (m *Metrics) IncDeliverySucceeded()          { m.deliverySucceeded.Add(1) }
func (m *Metrics) IncDeliveryFailed()             { m.deliveryFailed.Add(1) }
func (m *Metrics) IncDeliveryDeadLetter()         { m.deliveryDeadLetter.Add(1) }
func (m *Metrics) IncFunctionInvocations()        { m.functionInvocations.Add(1) }
func (m *Metrics) IncFunctionInvocationFailures() { m.functionInvocationFailures.Add(1) }
func (m *Metrics) IncResendWebhooksReceived()     { m.resendWebhooksReceived.Add(1) }
func (m *Metrics) IncResendWebhooksVerified()     { m.resendWebhooksVerified.Add(1) }
func (m *Metrics) IncResendWebhooksVerificationFailed() {
	m.resendVerificationFailed.Add(1)
}
func (m *Metrics) IncResendUnroutable()      { m.resendUnroutable.Add(1) }
func (m *Metrics) IncResendEventsPublished() { m.resendEventsPublished.Add(1) }
func (m *Metrics) IncResendEmailsSent()      { m.resendEmailsSent.Add(1) }
func (m *Metrics) IncSlackEventsReceived()   { m.slackEventsReceived.Add(1) }
func (m *Metrics) IncSlackThreadReplies()    { m.slackThreadReplies.Add(1) }
func (m *Metrics) IncStripeWebhooks()        { m.stripeWebhooks.Add(1) }
func (m *Metrics) IncStripeUnroutable()      { m.stripeUnroutable.Add(1) }
func (m *Metrics) IncNotionActions()         { m.notionActions.Add(1) }
func (m *Metrics) IncNotionActionFailures()  { m.notionActionFailures.Add(1) }
func (m *Metrics) IncLLMClassifications()    { m.llmClassifications.Add(1) }
func (m *Metrics) IncLLMExtractions()        { m.llmExtractions.Add(1) }
func (m *Metrics) IncLLMRequests(provider, operation string) {
	m.incLabelled(m.llmRequests, provider, operation)
}
func (m *Metrics) IncLLMFailures(provider string) {
	m.incLabelled(m.llmFailures, provider, "")
}
func (m *Metrics) ObserveLLMLatency(provider, operation string, seconds float64) {
	key := provider + "|" + operation
	m.mu.Lock()
	buckets := m.llmLatencyBuckets[key]
	if buckets == nil {
		buckets = make([]uint64, len(llmLatencyBounds)+1)
	}
	for i, bound := range llmLatencyBounds {
		if seconds <= bound {
			buckets[i]++
			m.llmLatencyBuckets[key] = buckets
			m.llmLatencySum[key] += seconds
			m.llmLatencyCount[key]++
			m.mu.Unlock()
			return
		}
	}
	buckets[len(llmLatencyBounds)]++
	m.llmLatencyBuckets[key] = buckets
	m.llmLatencySum[key] += seconds
	m.llmLatencyCount[key]++
	m.mu.Unlock()
}
func (m *Metrics) IncReplayRequests() { m.replayRequests.Add(1) }
func (m *Metrics) IncReplayJobsCreated(n int) {
	if n > 0 {
		m.replayJobsCreated.Add(uint64(n))
	}
}
func (m *Metrics) IncDeliveryRetries() { m.deliveryRetries.Add(1) }
func (m *Metrics) IncAgentRuns()       { m.agentRuns.Add(1) }
func (m *Metrics) IncAgentSteps()      { m.agentSteps.Add(1) }
func (m *Metrics) IncAgentToolCalls()  { m.agentToolCalls.Add(1) }
func (m *Metrics) IncSubscriptionFilterEvaluations() {
	m.subscriptionFilterEvaluations.Add(1)
}
func (m *Metrics) IncSubscriptionFilterMatches() {
	m.subscriptionFilterMatches.Add(1)
}
func (m *Metrics) IncSubscriptionFilterRejections() {
	m.subscriptionFilterRejections.Add(1)
}
func (m *Metrics) IncResultEventsEmitted(connector, operation, status string) {
	m.incCompositeLabelled(m.resultEventsEmitted, connector, operation, status)
}
func (m *Metrics) IncResultEventEmitFailures()  { m.resultEventEmitFailures.Add(1) }
func (m *Metrics) IncSchemaValidationFailures() { m.schemaValidationFailures.Add(1) }
func (m *Metrics) IncSchemaRegistered()         { m.schemaRegistered.Add(1) }
func (m *Metrics) IncSubscriptionTemplateValidationFailures() {
	m.subscriptionTemplateValidationFailures.Add(1)
}
func (m *Metrics) IncGraphRequests() { m.graphRequests.Add(1) }
func (m *Metrics) AddGraphNodes(n int) {
	if n > 0 {
		m.graphNodesTotal.Add(uint64(n))
	}
}
func (m *Metrics) AddGraphEdges(n int) {
	if n > 0 {
		m.graphEdgesTotal.Add(uint64(n))
	}
}
func (m *Metrics) IncGraphLimitExceeded() { m.graphLimitExceeded.Add(1) }
func (m *Metrics) SetEditionInfo(edition, tenancyMode string) {
	m.editionInfo.Store(EditionInfo{Edition: edition, TenancyMode: tenancyMode})
}
func (m *Metrics) SetLicenseInfo(edition string, present bool) {
	m.licenseInfo.Store(LicenseInfo{Edition: edition, LicensePresent: present})
}

func (m *Metrics) IncConnectorDeliveries(connector, operation string) {
	m.incLabelled(m.connectorDeliveries, connector, operation)
}

func (m *Metrics) IncConnectorDeliveryFailures(connector, operation string) {
	m.incLabelled(m.connectorDeliveryFailures, connector, operation)
}

func (m *Metrics) IncConnectorDeliveryDeadLetter(connector, operation string) {
	m.incLabelled(m.connectorDeliveryDeadLetter, connector, operation)
}

func (m *Metrics) IncInboundRoutes(connector string) {
	m.incLabelled(m.inboundRoutes, connector, "")
}

func (m *Metrics) IncInboundUnroutable(connector string) {
	m.incLabelled(m.inboundUnroutable, connector, "")
}

func (m *Metrics) IncGlobalConnectorDeliveries(connector, operation string) {
	m.incLabelled(m.globalConnectorDeliveries, connector, operation)
}

func (m *Metrics) Prometheus() string {
	lines := []string{
		fmt.Sprintf("groot_events_received_total %d", m.eventsReceived.Load()),
		fmt.Sprintf("groot_events_published_total %d", m.eventsPublished.Load()),
		fmt.Sprintf("groot_events_recorded_total %d", m.eventsRecorded.Load()),
		fmt.Sprintf("groot_router_events_consumed_total %d", m.routerEventsConsumed.Load()),
		fmt.Sprintf("groot_router_matches_total %d", m.routerMatches.Load()),
		fmt.Sprintf("groot_delivery_started_total %d", m.deliveryStarted.Load()),
		fmt.Sprintf("groot_delivery_succeeded_total %d", m.deliverySucceeded.Load()),
		fmt.Sprintf("groot_delivery_failed_total %d", m.deliveryFailed.Load()),
		fmt.Sprintf("groot_delivery_dead_letter_total %d", m.deliveryDeadLetter.Load()),
		fmt.Sprintf("groot_function_invocations_total %d", m.functionInvocations.Load()),
		fmt.Sprintf("groot_function_invocation_failures_total %d", m.functionInvocationFailures.Load()),
		fmt.Sprintf("groot_resend_webhooks_received_total %d", m.resendWebhooksReceived.Load()),
		fmt.Sprintf("groot_resend_webhooks_verified_total %d", m.resendWebhooksVerified.Load()),
		fmt.Sprintf("groot_resend_webhooks_verification_failed_total %d", m.resendVerificationFailed.Load()),
		fmt.Sprintf("groot_resend_unroutable_total %d", m.resendUnroutable.Load()),
		fmt.Sprintf("groot_resend_events_published_total %d", m.resendEventsPublished.Load()),
		fmt.Sprintf("groot_resend_emails_sent_total %d", m.resendEmailsSent.Load()),
		fmt.Sprintf("groot_slack_events_received_total %d", m.slackEventsReceived.Load()),
		fmt.Sprintf("groot_slack_thread_replies_total %d", m.slackThreadReplies.Load()),
		fmt.Sprintf("groot_stripe_webhooks_total %d", m.stripeWebhooks.Load()),
		fmt.Sprintf("groot_stripe_unroutable_total %d", m.stripeUnroutable.Load()),
		fmt.Sprintf("groot_notion_actions_total %d", m.notionActions.Load()),
		fmt.Sprintf("groot_notion_action_failures_total %d", m.notionActionFailures.Load()),
		fmt.Sprintf("groot_llm_classifications_total %d", m.llmClassifications.Load()),
		fmt.Sprintf("groot_llm_extractions_total %d", m.llmExtractions.Load()),
		fmt.Sprintf("groot_replay_requests_total %d", m.replayRequests.Load()),
		fmt.Sprintf("groot_replay_jobs_created_total %d", m.replayJobsCreated.Load()),
		fmt.Sprintf("groot_delivery_retries_total %d", m.deliveryRetries.Load()),
		fmt.Sprintf("groot_agent_runs_total %d", m.agentRuns.Load()),
		fmt.Sprintf("groot_agent_steps_total %d", m.agentSteps.Load()),
		fmt.Sprintf("groot_agent_tool_calls_total %d", m.agentToolCalls.Load()),
		fmt.Sprintf("groot_subscription_filter_evaluations_total %d", m.subscriptionFilterEvaluations.Load()),
		fmt.Sprintf("groot_subscription_filter_matches_total %d", m.subscriptionFilterMatches.Load()),
		fmt.Sprintf("groot_subscription_filter_rejections_total %d", m.subscriptionFilterRejections.Load()),
		fmt.Sprintf("groot_result_event_emit_failures_total %d", m.resultEventEmitFailures.Load()),
		fmt.Sprintf("groot_schema_validation_failures_total %d", m.schemaValidationFailures.Load()),
		fmt.Sprintf("groot_schema_registered_total %d", m.schemaRegistered.Load()),
		fmt.Sprintf("groot_subscription_template_validation_failures_total %d", m.subscriptionTemplateValidationFailures.Load()),
		fmt.Sprintf("groot_graph_requests_total %d", m.graphRequests.Load()),
		fmt.Sprintf("groot_graph_nodes_total %d", m.graphNodesTotal.Load()),
		fmt.Sprintf("groot_graph_edges_total %d", m.graphEdgesTotal.Load()),
		fmt.Sprintf("groot_graph_limit_exceeded_total %d", m.graphLimitExceeded.Load()),
	}
	lines = append(lines, m.labelledPrometheus("groot_connector_deliveries_total", m.snapshot(m.connectorDeliveries))...)
	lines = append(lines, m.labelledPrometheus("groot_connector_delivery_failures_total", m.snapshot(m.connectorDeliveryFailures))...)
	lines = append(lines, m.labelledPrometheus("groot_connector_delivery_dead_letter_total", m.snapshot(m.connectorDeliveryDeadLetter))...)
	lines = append(lines, m.labelledPrometheus("groot_inbound_routes_total", m.snapshot(m.inboundRoutes))...)
	lines = append(lines, m.labelledPrometheus("groot_inbound_unroutable_total", m.snapshot(m.inboundUnroutable))...)
	lines = append(lines, m.labelledPrometheus("groot_global_connector_deliveries_total", m.snapshot(m.globalConnectorDeliveries))...)
	lines = append(lines, m.providerOperationPrometheus("groot_llm_requests_total", m.snapshot(m.llmRequests))...)
	lines = append(lines, m.providerPrometheus("groot_llm_failures_total", m.snapshot(m.llmFailures))...)
	lines = append(lines, m.resultEventPrometheus("groot_result_events_emitted_total", m.snapshot(m.resultEventsEmitted))...)
	lines = append(lines, m.llmLatencyPrometheus()...)
	if info, ok := m.editionInfo.Load().(EditionInfo); ok && info.Edition != "" {
		lines = append(lines, fmt.Sprintf("groot_edition_info{edition=%q,tenancy_mode=%q} 1", info.Edition, info.TenancyMode))
	}
	if info, ok := m.licenseInfo.Load().(LicenseInfo); ok && info.Edition != "" {
		lines = append(lines, fmt.Sprintf("groot_license_info{edition=%q,license_present=%q} 1", info.Edition, strconv.FormatBool(info.LicensePresent)))
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *Metrics) incLabelled(target map[string]uint64, connector, operation string) {
	key := connector + "|" + operation
	m.mu.Lock()
	target[key]++
	m.mu.Unlock()
}

func (m *Metrics) snapshot(target map[string]uint64) map[string]uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]uint64, len(target))
	for key, value := range target {
		result[key] = value
	}
	return result
}

func (m *Metrics) incCompositeLabelled(target map[string]uint64, first, second, third string) {
	key := first + "|" + second + "|" + third
	m.mu.Lock()
	target[key]++
	m.mu.Unlock()
}

func (m *Metrics) llmLatencyPrometheus() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.llmLatencyBuckets))
	for key := range m.llmLatencyBuckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)*(len(llmLatencyBounds)+3))
	for _, key := range keys {
		parts := strings.SplitN(key, "|", 2)
		provider, operation := parts[0], ""
		if len(parts) == 2 {
			operation = parts[1]
		}
		cumulative := uint64(0)
		for i, bound := range llmLatencyBounds {
			cumulative += m.llmLatencyBuckets[key][i]
			lines = append(lines, fmt.Sprintf("groot_llm_latency_seconds_bucket{provider=%q,operation=%q,le=%q} %d", provider, operation, strconv.FormatFloat(bound, 'f', -1, 64), cumulative))
		}
		cumulative += m.llmLatencyBuckets[key][len(llmLatencyBounds)]
		lines = append(lines, fmt.Sprintf("groot_llm_latency_seconds_bucket{provider=%q,operation=%q,le=%q} %d", provider, operation, "+Inf", cumulative))
		lines = append(lines, fmt.Sprintf("groot_llm_latency_seconds_sum{provider=%q,operation=%q} %g", provider, operation, m.llmLatencySum[key]))
		lines = append(lines, fmt.Sprintf("groot_llm_latency_seconds_count{provider=%q,operation=%q} %d", provider, operation, m.llmLatencyCount[key]))
	}
	return lines
}

func (m *Metrics) labelledPrometheus(metricName string, values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, "|", 2)
		connector, operation := parts[0], ""
		if len(parts) == 2 {
			operation = parts[1]
		}
		lines = append(lines, fmt.Sprintf("%s{connector=%q,operation=%q} %d", metricName, connector, operation, values[key]))
	}
	return lines
}

func (m *Metrics) providerPrometheus(metricName string, values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		provider := strings.SplitN(key, "|", 2)[0]
		lines = append(lines, fmt.Sprintf("%s{provider=%q} %d", metricName, provider, values[key]))
	}
	return lines
}

func (m *Metrics) providerOperationPrometheus(metricName string, values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, "|", 2)
		provider, operation := parts[0], ""
		if len(parts) == 2 {
			operation = parts[1]
		}
		lines = append(lines, fmt.Sprintf("%s{provider=%q,operation=%q} %d", metricName, provider, operation, values[key]))
	}
	return lines
}

func (m *Metrics) resultEventPrometheus(metricName string, values map[string]uint64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, "|", 3)
		connector, operation, status := "", "", ""
		if len(parts) > 0 {
			connector = parts[0]
		}
		if len(parts) > 1 {
			operation = parts[1]
		}
		if len(parts) > 2 {
			status = parts[2]
		}
		lines = append(lines, fmt.Sprintf("%s{connector=%q,operation=%q,status=%q} %d", metricName, connector, operation, status, values[key]))
	}
	return lines
}
