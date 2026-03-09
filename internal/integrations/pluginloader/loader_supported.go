//go:build linux || darwin || freebsd

package pluginloader

import (
	"fmt"
	pluginpkg "plugin"

	sdkintegration "groot/sdk/integration"

	"groot/internal/integrations"
)

type pluginSymbolResolver interface {
	Lookup(string) (pluginpkg.Symbol, error)
}

type stdlibOpener struct{}

func (stdlibOpener) Open(path string) (integration.Integration, error) {
	return Open(path)
}

func Open(path string) (integration.Integration, error) {
	plug, err := pluginpkg.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plugin: %w", err)
	}
	return resolveIntegrationSymbol(plug)
}

func resolveIntegrationSymbol(resolver pluginSymbolResolver) (integration.Integration, error) {
	symbol, err := resolver.Lookup("Integration")
	if err != nil {
		return nil, fmt.Errorf("lookup Integration symbol: %w", err)
	}
	exported, ok := symbol.(*sdkintegration.Integration)
	if !ok {
		return nil, fmt.Errorf("Integration symbol has wrong type")
	}
	if exported == nil || *exported == nil {
		return nil, fmt.Errorf("Integration symbol is nil")
	}
	return sdkAdapter{plugin: *exported}, nil
}
