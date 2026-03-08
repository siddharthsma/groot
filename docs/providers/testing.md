# Provider Testing

Every provider must have a local `provider_test.go` that runs the shared conformance harness:

```go
func TestProviderConformance(t *testing.T) {
    providertests.RunProviderTests(t, Provider{})
}
```

## What The Conformance Harness Checks

- provider spec is structurally valid
- provider is registered in the central registry
- config fields are declared
- schema declarations are present and valid JSON
- `ValidateConfig` runs without panicking

## Additional Tests

Conformance is the floor, not the full test plan.

Providers should also keep package-local tests for:

- outbound request building
- inbound verification
- retryable vs permanent error handling
- response parsing
- provider-specific edge cases

Integration scenarios may still live elsewhere in the repo when they need the full stack.
