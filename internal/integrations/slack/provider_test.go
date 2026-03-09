package slack

import (
	"testing"

	integrationtests "groot/internal/integrations/testsuite"
)

func TestIntegrationConformance(t *testing.T) {
	integrationtests.RunIntegrationTests(t, Integration{})
}
