//go:build !linux && !darwin && !freebsd

package pluginloader

import (
	"fmt"

	"groot/internal/connectors/provider"
)

type stdlibOpener struct{}

func (stdlibOpener) Open(path string) (provider.Provider, error) {
	return Open(path)
}

func Open(string) (provider.Provider, error) {
	return nil, fmt.Errorf("go plugins are not supported on %s", runtimeGOOS())
}

func runtimeGOOS() string {
	return "this platform"
}
