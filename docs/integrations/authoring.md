# Integration Authoring

## Steps

1. Run `scripts/new-integration.sh <name>`.
2. Fill in `integration.go` with the integration spec and `init()` registration.
3. Define config shapes in `config.go`.
4. Implement `ValidateConfig(map[string]any)` in `validate.go`.
5. Implement outbound operations in `operations.go`.
6. Implement inbound handling in `inbound.go` if the integration receives external webhooks.
7. Declare every integration-owned schema in `schemas.go`.
8. Add package-local conformance coverage in `integration_test.go`.
9. Add richer behavior tests as needed.
10. Update integration `README.md`.

## Validation Model

Integrations validate in two layers:

- registry/spec validation checks the declared integration metadata
- integration-internal validation decodes `map[string]any` into typed config and normalizes it for storage

`ValidateConfig` should:

- reject malformed config
- trim or normalize values where needed
- mark secret fields in the integration spec
- never log secrets

## Scope Rules

Declare whether the integration supports:

- tenant scope
- global scope

Do not rely on ad-hoc checks outside the integration spec unless the behavior is a deliberate higher-level policy such as edition gating or global-instance toggles.

## Schema Ownership

Every event schema directly owned by the integration must be declared in `schemas.go`.

Use:

- external source kind for inbound events
- internal source kind for result events

Non-integration schemas that belong to core Groot behavior stay in `internal/schema`.
