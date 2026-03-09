//go:build linux || darwin || freebsd

package pluginloader

import (
	"context"
	"encoding/json"
	pluginpkg "plugin"
	"testing"

	sdkintegration "groot/sdk/integration"
)

type fakeResolver struct {
	symbol any
	err    error
}

func (f fakeResolver) Lookup(string) (pluginpkg.Symbol, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.symbol, nil
}

type fakeSDKIntegration struct{}

func (fakeSDKIntegration) Spec() sdkintegration.IntegrationSpec {
	return sdkintegration.IntegrationSpec{
		Name:                "plugin_test",
		SupportsTenantScope: true,
		Config: sdkintegration.ConfigSpec{
			Fields: []sdkintegration.ConfigField{{Name: "token"}},
		},
		Operations: []sdkintegration.OperationSpec{{Name: "echo", Description: "echo"}},
	}
}

func (fakeSDKIntegration) ValidateConfig(map[string]any) error { return nil }

func (fakeSDKIntegration) ExecuteOperation(context.Context, sdkintegration.OperationRequest) (sdkintegration.OperationResult, error) {
	return sdkintegration.OperationResult{Output: json.RawMessage(`{"ok":true}`)}, nil
}

func TestResolveIntegrationSymbol(t *testing.T) {
	var exported sdkintegration.Integration = fakeSDKIntegration{}
	resolved, err := resolveIntegrationSymbol(fakeResolver{symbol: &exported})
	if err != nil {
		t.Fatalf("resolveIntegrationSymbol() error = %v", err)
	}
	if resolved.Spec().Name != "plugin_test" {
		t.Fatalf("name = %q", resolved.Spec().Name)
	}
}

func TestResolveIntegrationSymbolWrongType(t *testing.T) {
	_, err := resolveIntegrationSymbol(fakeResolver{symbol: "bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}
