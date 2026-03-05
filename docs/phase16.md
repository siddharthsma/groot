
# Groot — Phase 16

## Goal

Add payload-based subscription filtering so subscribers can trigger on:

- event type
- AND optional matches on fields within the event payload

Filtering must be:

- deterministic
- schema-aware (Phase 14)
- safe (no arbitrary code)
- fast enough for Phase 16 scale (in-process evaluation)

No UI.

---

# Scope

Phase 16 implements:

1. Subscription filter model (`filter_json`)
2. Filter grammar (AND/OR/NOT + simple operators)
3. Schema-aware validation at subscription create/update
4. Router evaluation of filters before enqueuing delivery jobs
5. Observability for filter evaluation
6. Tests covering correctness and validation

---

# Database

Create migration:

migrations/016_subscription_filters.sql

Add column:

ALTER TABLE subscriptions
ADD COLUMN filter_json JSONB;

Index (optional):

CREATE INDEX subscriptions_filter_json_gin
ON subscriptions
USING GIN (filter_json);

---

# Filter Grammar

Filters are JSON objects.

Top-level:

- `all`: array of conditions (AND)
- `any`: array of conditions (OR)
- `not`: a single filter (NOT)

Condition object:

{ "path": "payload.amount", "op": ">=", "value": 100 }

Supported operators:

- `==`, `!=`
- `>`, `>=`, `<`, `<=`
- `contains`
- `in`
- `exists`

Rules:

- exactly one of `all|any|not` OR a condition object
- empty `all` or `any` invalid
- max nesting depth = 10
- max conditions per subscription = 50

---

# Path Rules

- paths must start with `payload.`
- dot-separated object traversal only
- arrays not supported in Phase 16

Allowed:

payload.subject
payload.customer.email

Rejected:

payload.items[0].sku
payload.items[*].sku

---

# Schema-Aware Validation

Validation occurs on:

- POST /subscriptions
- PUT /subscriptions/{id}

Steps:

1. Parse filter_json into AST.
2. Validate grammar constraints.
3. Load schema for subscription.event_type.
4. If schema missing:
   - allow subscription
   - return warning
5. If schema exists:
   - validate path exists
   - validate operator compatible with field type

Invalid filters return HTTP 400 with:

- invalid_paths
- invalid_ops

---

# Router Evaluation

Router logic:

1. Match subscriptions by event_type
2. If filter_json null → match
3. Else evaluate filter against event.payload
4. Only matching subscriptions create delivery_jobs

Evaluation rules:

- missing path → false
- exists op → true only if path present

---

# API Changes

Subscription requests accept:

{
  "filter": { ... }
}

Stored as filter_json.

Responses may include warnings:

{
  "warnings": ["schema_missing_for_event_type"]
}

---

# Observability

Logs:

subscription_filter_evaluated
subscription_filter_invalid
subscription_filter_schema_missing

Metrics:

groot_subscription_filter_evaluations_total
groot_subscription_filter_matches_total
groot_subscription_filter_rejections_total

---

# Tests

Test 1 — Equality

filter: payload.currency == "usd"

Test 2 — Numeric

filter: payload.amount >= 100

Test 3 — Nested

all[currency=="usd", any[amount>=100, vip==true]]

Test 4 — Schema validation

invalid path rejected

Test 5 — Operator validation

invalid operator rejected

Test 6 — Missing schema

subscription allowed with warning

Rollback:

truncate subscriptions, events, delivery_jobs for test tenant

---

# Phase 16 Completion Criteria

- filter_json column exists
- filter grammar validated
- schema-aware validation implemented
- router evaluates filters correctly
- observability logs and metrics exist
- tests cover matching, nesting, and validation
