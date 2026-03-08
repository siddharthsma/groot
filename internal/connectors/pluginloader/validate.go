package pluginloader

import (
	"fmt"
	"strings"

	"groot/internal/connectors/provider"
)

func validatePluginSpec(spec provider.ProviderSpec) error {
	if err := provider.ValidateSpec(spec); err != nil {
		return err
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("provider name is required")
	}
	return nil
}
