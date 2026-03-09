package testsuite

import (
	"testing"

	"groot/internal/integrations"
	"groot/internal/integrations/registry"
)

func RunIntegrationTests(t *testing.T, p integration.Integration) {
	t.Helper()

	if p == nil {
		t.Fatal("integration is nil")
	}
	spec := p.Spec()
	if err := integration.ValidateSpec(spec); err != nil {
		t.Fatalf("ValidateSpec() error = %v", err)
	}
	registered := registry.GetIntegration(spec.Name)
	if registered == nil {
		t.Fatalf("integration %s is not registered", spec.Name)
	}
	if registered.Spec().Name != spec.Name {
		t.Fatalf("registered integration name = %q, want %q", registered.Spec().Name, spec.Name)
	}
	config := map[string]any{}
	err := p.ValidateConfig(config)
	if len(spec.Config.Fields) == 0 && err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
}
