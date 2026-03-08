//go:build linux || darwin || freebsd

package pluginloader

import (
	"context"
	"encoding/json"
	pluginpkg "plugin"
	"testing"

	sdkprovider "groot/sdk/provider"
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

type fakeSDKProvider struct{}

func (fakeSDKProvider) Spec() sdkprovider.ProviderSpec {
	return sdkprovider.ProviderSpec{
		Name:                "plugin_test",
		SupportsTenantScope: true,
		Config: sdkprovider.ConfigSpec{
			Fields: []sdkprovider.ConfigField{{Name: "token"}},
		},
		Operations: []sdkprovider.OperationSpec{{Name: "echo", Description: "echo"}},
	}
}

func (fakeSDKProvider) ValidateConfig(map[string]any) error { return nil }

func (fakeSDKProvider) ExecuteOperation(context.Context, sdkprovider.OperationRequest) (sdkprovider.OperationResult, error) {
	return sdkprovider.OperationResult{Output: json.RawMessage(`{"ok":true}`)}, nil
}

func TestResolveProviderSymbol(t *testing.T) {
	var exported sdkprovider.Provider = fakeSDKProvider{}
	resolved, err := resolveProviderSymbol(fakeResolver{symbol: &exported})
	if err != nil {
		t.Fatalf("resolveProviderSymbol() error = %v", err)
	}
	if resolved.Spec().Name != "plugin_test" {
		t.Fatalf("name = %q", resolved.Spec().Name)
	}
}

func TestResolveProviderSymbolWrongType(t *testing.T) {
	_, err := resolveProviderSymbol(fakeResolver{symbol: "bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}
