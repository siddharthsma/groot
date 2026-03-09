//go:build !linux && !darwin && !freebsd

package pluginloader

import (
	"fmt"

	"groot/internal/integrations"
)

type stdlibOpener struct{}

func (stdlibOpener) Open(path string) (integration.Integration, error) {
	return Open(path)
}

func Open(string) (integration.Integration, error) {
	return nil, fmt.Errorf("go plugins are not supported on %s", runtimeGOOS())
}

func runtimeGOOS() string {
	return "this platform"
}
