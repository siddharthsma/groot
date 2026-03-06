package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	eventsReceived             atomic.Uint64
	eventsPublished            atomic.Uint64
	eventsRecorded             atomic.Uint64
	routerEventsConsumed       atomic.Uint64
	routerMatches              atomic.Uint64
	deliveryStarted            atomic.Uint64
	deliverySucceeded          atomic.Uint64
	deliveryFailed             atomic.Uint64
	deliveryDeadLetter         atomic.Uint64
	functionInvocations        atomic.Uint64
	functionInvocationFailures atomic.Uint64
	resendWebhooksReceived     atomic.Uint64
	resendWebhooksVerified     atomic.Uint64
	resendVerificationFailed   atomic.Uint64
	resendUnroutable           atomic.Uint64
	resendEventsPublished      atomic.Uint64

	mu                          sync.Mutex
	connectorDeliveries         map[string]uint64
	connectorDeliveryFailures   map[string]uint64
	connectorDeliveryDeadLetter map[string]uint64
	inboundRoutes               map[string]uint64
	inboundUnroutable           map[string]uint64
	globalConnectorDeliveries   map[string]uint64
}

func NewMetrics() *Metrics {
	return &Metrics{
		connectorDeliveries:         make(map[string]uint64),
		connectorDeliveryFailures:   make(map[string]uint64),
		connectorDeliveryDeadLetter: make(map[string]uint64),
		inboundRoutes:               make(map[string]uint64),
		inboundUnroutable:           make(map[string]uint64),
		globalConnectorDeliveries:   make(map[string]uint64),
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
	}
	lines = append(lines, m.labelledPrometheus("groot_connector_deliveries_total", m.snapshot(m.connectorDeliveries))...)
	lines = append(lines, m.labelledPrometheus("groot_connector_delivery_failures_total", m.snapshot(m.connectorDeliveryFailures))...)
	lines = append(lines, m.labelledPrometheus("groot_connector_delivery_dead_letter_total", m.snapshot(m.connectorDeliveryDeadLetter))...)
	lines = append(lines, m.labelledPrometheus("groot_inbound_routes_total", m.snapshot(m.inboundRoutes))...)
	lines = append(lines, m.labelledPrometheus("groot_inbound_unroutable_total", m.snapshot(m.inboundUnroutable))...)
	lines = append(lines, m.labelledPrometheus("groot_global_connector_deliveries_total", m.snapshot(m.globalConnectorDeliveries))...)
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
