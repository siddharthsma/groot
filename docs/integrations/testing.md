# Integration Testing

Every integration must have a local `integration_test.go` that runs the shared conformance harness:

```go
func TestIntegrationConformance(t *testing.T) {
    integrationtests.RunIntegrationTests(t, Integration{})
}
```

## What The Conformance Harness Checks

- integration spec is structurally valid
- integration is registered in the central registry
- config fields are declared
- schema declarations are present and valid JSON
- `ValidateConfig` runs without panicking

## Additional Tests

Conformance is the floor, not the full test plan.

Integrations should also keep package-local tests for:

- outbound request building
- inbound verification
- retryable vs permanent error handling
- response parsing
- integration-specific edge cases

Integration scenarios may still live elsewhere in the repo when they need the full stack.
