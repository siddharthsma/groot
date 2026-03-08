//go:build linux || darwin || freebsd

package pluginloader

import (
	"fmt"
	pluginpkg "plugin"

	sdkprovider "groot/sdk/provider"

	"groot/internal/connectors/provider"
)

type pluginSymbolResolver interface {
	Lookup(string) (pluginpkg.Symbol, error)
}

type stdlibOpener struct{}

func (stdlibOpener) Open(path string) (provider.Provider, error) {
	return Open(path)
}

func Open(path string) (provider.Provider, error) {
	plug, err := pluginpkg.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plugin: %w", err)
	}
	return resolveProviderSymbol(plug)
}

func resolveProviderSymbol(resolver pluginSymbolResolver) (provider.Provider, error) {
	symbol, err := resolver.Lookup("Provider")
	if err != nil {
		return nil, fmt.Errorf("lookup Provider symbol: %w", err)
	}
	exported, ok := symbol.(*sdkprovider.Provider)
	if !ok {
		return nil, fmt.Errorf("Provider symbol has wrong type")
	}
	if exported == nil || *exported == nil {
		return nil, fmt.Errorf("Provider symbol is nil")
	}
	return sdkAdapter{plugin: *exported}, nil
}
