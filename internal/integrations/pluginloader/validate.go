package pluginloader

import (
	"fmt"
	"strings"

	"groot/internal/integrations"
)

func validatePluginSpec(spec integration.IntegrationSpec) error {
	if err := integration.ValidateSpec(spec); err != nil {
		return err
	}
	if strings.TrimSpace(spec.Name) == "" {
		return fmt.Errorf("integration name is required")
	}
	return nil
}
