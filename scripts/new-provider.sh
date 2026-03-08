#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: scripts/new-provider.sh <provider_name>" >&2
  exit 1
fi

name="$1"
dir="internal/connectors/providers/$name"

if [[ -e "$dir" ]]; then
  echo "provider already exists: $dir" >&2
  exit 1
fi

mkdir -p "$dir"

cat >"$dir/provider.go" <<EOF
package $name

import (
	"context"

	"groot/internal/connectors/provider"
	"groot/internal/connectors/registry"
)

type Provider struct{}

func init() {
	registry.RegisterProvider(Provider{})
}

func (Provider) Spec() provider.ProviderSpec {
	return provider.ProviderSpec{
		Name:                "$name",
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config:              provider.ConfigSpec{},
		Operations:          []provider.OperationSpec{},
		Schemas:             Schemas(),
	}
}

func (Provider) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Provider) ExecuteOperation(context.Context, provider.OperationRequest) (provider.OperationResult, error) {
	return provider.OperationResult{}, nil
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

import "groot/internal/connectors/provider"

func Schemas() []provider.SchemaSpec {
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

	providertests "groot/internal/connectors/provider/testsuite"
)

func TestProviderConformance(t *testing.T) {
	providertests.RunProviderTests(t, Provider{})
}
EOF

cat >"$dir/README.md" <<EOF
# ${name^} Provider

## Purpose

## Supported Scopes

## Inbound Events

## Outbound Operations

## Required Config

## Secrets

## Testing
EOF

echo "created $dir"
