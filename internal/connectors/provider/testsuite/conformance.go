package testsuite

import (
	"testing"

	"groot/internal/connectors/provider"
	"groot/internal/connectors/registry"
)

func RunProviderTests(t *testing.T, p provider.Provider) {
	t.Helper()

	if p == nil {
		t.Fatal("provider is nil")
	}
	spec := p.Spec()
	if err := provider.ValidateSpec(spec); err != nil {
		t.Fatalf("ValidateSpec() error = %v", err)
	}
	registered := registry.GetProvider(spec.Name)
	if registered == nil {
		t.Fatalf("provider %s is not registered", spec.Name)
	}
	if registered.Spec().Name != spec.Name {
		t.Fatalf("registered provider name = %q, want %q", registered.Spec().Name, spec.Name)
	}
	config := map[string]any{}
	err := p.ValidateConfig(config)
	if len(spec.Config.Fields) == 0 && err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
