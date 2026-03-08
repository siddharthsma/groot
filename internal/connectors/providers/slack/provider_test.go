package slack

import (
	"testing"

	providertests "groot/internal/connectors/provider/testsuite"
)

func TestProviderConformance(t *testing.T) {
	providertests.RunProviderTests(t, Provider{})
}
