#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: scripts/new-integration.sh <integration_name>" >&2
  exit 1
fi

name="$1"
dir="internal/integrations/$name"

if [[ -e "$dir" ]]; then
  echo "integration already exists: $dir" >&2
  exit 1
fi

mkdir -p "$dir"

cat >"$dir/integration.go" <<EOF
package $name

import (
	"context"

	"groot/internal/integrations"
	"groot/internal/integrations/registry"
)

type Integration struct{}

func init() {
	registry.RegisterIntegration(Integration{})
}

func (Integration) Spec() integration.IntegrationSpec {
	return integration.IntegrationSpec{
		Name:                "$name",
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config:              integration.ConfigSpec{},
		Operations:          []integration.OperationSpec{},
		Schemas:             Schemas(),
	}
}

func (Integration) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Integration) ExecuteOperation(context.Context, integration.OperationRequest) (integration.OperationResult, error) {
	return integration.OperationResult{}, nil
}
EOF

cat >"$dir/config.go" <<EOF
package $name
EOF

cat >"$dir/inbound.go" <<EOF
package $name
EOF

cat >"$dir/operations.go" <<EOF
package $name
EOF

cat >"$dir/schemas.go" <<EOF
package $name

import "groot/internal/integrations"

func Schemas() []integration.SchemaSpec {
	return nil
}
EOF

cat >"$dir/validate.go" <<EOF
package $name

func validateConfig(config map[string]any) error {
	return nil
}
EOF

cat >"$dir/provider_test.go" <<EOF
package $name

import (
	"testing"

	integrationtests "groot/internal/integrations/testsuite"
)

func TestIntegrationConformance(t *testing.T) {
	integrationtests.RunIntegrationTests(t, Integration{})
}
EOF

cat >"$dir/README.md" <<EOF
# ${name^} Integration

## Purpose

## Supported Scopes

## Inbound Events

## Outbound Operations

## Required Config

## Secrets

## Testing
EOF

echo "created $dir"
