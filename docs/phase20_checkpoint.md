
# Groot — Phase 20 (Checkpoint)

## Goal

Perform a full checkpoint audit of Groot through the end of Phase 19.

Phase 20 is not about new product features.
It is about proving that the system is:

- correctly implemented
- internally consistent
- integration-tested end-to-end
- stable enough to proceed to UI work

---

# Scope

Phase 20 implements:

1. Full integration test suite for Phases 0–19
2. Cross-phase audit of schema, APIs, workers, and routing behavior
3. Deterministic local test environment
4. Regression coverage for critical event chains
5. Replay, agent, schema, filter, and graph validation tests
6. Make targets for repeatable checkpoint execution
7. Cleanup and rollback guarantees for all tests

No UI.
No new product functionality except testability helpers required to make checkpoint execution deterministic.

---

# Checkpoint Principles

All checkpoint tests must be:

- local
- deterministic
- isolated
- repeatable
- automated

Tests must not depend on:

- real third-party SaaS accounts
- public internet services
- manual setup beyond documented local bootstrap

All external integrations must be simulated with local test servers or mocks.

---

# Required Test Environment

Phase 20 assumes the local stack includes:

- PostgreSQL
- Kafka / event stream infrastructure
- Temporal
- Groot API service
- Groot router
- Groot delivery worker

Optional local mock services must be added for checkpoint tests:

- mock Slack server
- mock Notion server
- mock Resend API/webhook sender
- mock Stripe webhook sender
- mock function destination endpoint
- mock JWKS server
- mock LLM integration server

Preferred approach:
- in-test Go HTTP servers where possible

---

# Make Targets

Add the following targets:

make checkpoint
make checkpoint-fast
make checkpoint-integration
make checkpoint-reset
make checkpoint-audit

---

# Test Layout

Create directory structure:

/tests
  /integration
  /helpers

Integration tests should cover phases 01–19 with dedicated files for each phase.

---

# Rollback / Cleanup Requirements

Every integration test must leave the system clean.

Use:

- DB truncation for all mutable tables
- isolated Kafka topics or unique event types per test
- isolated Temporal workflow IDs
- teardown of all mock servers

No test may rely on state from a previous test.

---

# Required Mock/Fixture Support

Provide reusable fixtures for:

- Slack
- Notion
- Resend
- Stripe
- LLM
- Function destinations
- JWKS/JWT

Mocks must support success, retryable failure, and permanent failure scenarios.

---

# Audit Checks

Phase 20 must include automated audits for:

- migration order and completeness
- API route registration
- environment configuration parsing
- documentation presence (README, AGENTS.md)
- required Make targets

---

# Golden End‑to‑End Scenarios

## Scenario 1 — Email triage

Resend inbound → LLM classify → filter → Slack post message → result event

## Scenario 2 — Support agent

Slack inbound → llm.agent → notion.create_page → slack.create_thread_reply

## Scenario 3 — Replay and graph

Event chain → replay → execution graph inspection

---

# Stability Gates

Phase 20 passes only if:

1. all migrations apply cleanly from empty DB
2. make checkpoint exits 0
3. integration tests pass
4. tests run successfully multiple times without flakiness
5. replay, agent, result events, graph APIs, and auth flows work end‑to‑end
6. admin APIs enforce payload redaction by default
7. no secrets appear in logs or responses during tests
8. audit logs exist for all tested resource mutations

---

# Deliverables

Phase 20 must produce:

1. integration test suite under /tests/integration
2. reusable mock servers and fixtures
3. checkpoint Make targets
4. documentation updates
5. generated audit report:

artifacts/phase20_audit_report.md

---

# Phase 20 Completion Criteria

All conditions must be met:

- integration suite exists and passes locally
- checkpoint Make targets succeed
- rollback/cleanup deterministic
- golden scenarios pass
- audit report generated
