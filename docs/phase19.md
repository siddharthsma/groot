
# Groot — Phase 19

## Goal

Add an Operator Graph API that exposes:

1. system topology (what can happen)
2. per-event execution graph (what did happen)

APIs are admin-only and used by the upcoming Operator UI.

No UI in Phase 19.

---

# Scope

Phase 19 implements:

1. Graph domain model (nodes + edges) with stable IDs
2. `/admin/topology` endpoint (derived from DB state)
3. `/admin/events/{event_id}/execution-graph` endpoint (derived from events + delivery jobs + result events)
4. Minimal filtering (tenant, connector, event type prefix)
5. Observability + limits for graph construction
6. Tests for correctness and safety

---

# Graph Model

Create package:

internal/graph

## Node

{
  "id": "string",
  "type": "string",
  "label": "string",
  "tenant_id": "uuid|null",
  "data": {}
}

## Edge

{
  "from": "string",
  "to": "string",
  "type": "string",
  "data": {}
}

## Node types

connector_instance
event_type
subscription
event
delivery_job

## Edge types

emits
triggers
delivers_to
triggered
emitted

---

# Stable ID Format

connection: conninst:<uuid>

event type: eventtype:<full_name>

subscription: sub:<uuid>

event: evt:<uuid>

delivery job: job:<uuid>

---

# Limits

Add env vars:

GRAPH_MAX_NODES=5000
GRAPH_MAX_EDGES=20000
GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS=500
GRAPH_EXECUTION_MAX_DEPTH=25
GRAPH_DEFAULT_LIMIT=500

Rules:

- if limits exceeded return error graph_too_large
- traversal uses visited set and depth cap
- payloads never included in graph responses

---

# Admin Endpoints

Require Phase 18 admin auth.

---

## Topology Graph

GET /admin/topology

Query params:

tenant_id
connector_name
event_type_prefix
include_global=true|false
limit

Derivation:

Nodes from:

connector_instances
event_schemas
subscriptions

Edges:

connector_instance → event_type (emits)
event_type → subscription (triggers)
subscription → connector_instance (delivers_to)

---

## Execution Graph

GET /admin/events/{event_id}/execution-graph

Query params:

max_depth
max_events

Traversal:

1. start from event
2. find delivery_jobs where delivery_jobs.event_id = event_id
3. add delivery_job nodes + triggered edges
4. find result events from delivery_jobs.result_event_id
5. add emitted edges
6. continue traversal until limits reached

Tenant isolation enforced.

Response:

{
  "event_id": "uuid",
  "nodes": [],
  "edges": [],
  "summary": {
    "status": "succeeded|failed|partial",
    "duration_ms": 0,
    "jobs_total": 0,
    "jobs_failed": 0
  }
}

---

# Storage Requirements

delivery_jobs must contain:

event_id
subscription_id
result_event_id

If result_event_id missing fallback to events envelope lookup.

---

# Observability

Logs:

graph_topology_built
graph_execution_built
graph_limit_exceeded

Metrics:

groot_graph_requests_total
groot_graph_nodes_total
groot_graph_edges_total
groot_graph_limit_exceeded_total

---

# Tests

Test 1 — Topology edges

Setup connectors, schemas, subscriptions.

Verify emits + triggers edges.

---

Test 2 — Tenant filter

Topology query with tenant_id excludes other tenants.

---

Test 3 — Execution chain

event → job → event → job

Verify nodes and edges created correctly.

---

Test 4 — Execution limits

Graph exceeding depth returns partial graph or error.

Rollback:

truncate events, delivery_jobs, subscriptions, connector_instances

---

# Phase 19 Completion Criteria

- internal/graph package implemented
- /admin/topology endpoint implemented
- /admin/events/{event_id}/execution-graph endpoint implemented
- graph limits enforced
- no payloads exposed
- logs and metrics implemented
- tests validate topology and execution graph correctness
