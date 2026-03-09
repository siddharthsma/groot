# Integration Packages

Phase 30 introduces signed integration packages for installing external integrations outside the core repository.

## Package Format

Integration packages use the `.grootpkg` extension.

They are tar archives with this layout:

```text
integration/
  integration.so
  manifest.json
  signature.ed25519
```

## Manifest

`manifest.json` describes the package:

```json
{
  "name": "customcrm",
  "version": "1.0.0",
  "description": "Custom CRM integration",
  "author": "Example Corp",
  "groot_version": ">=1.0.0",
  "integration_spec_hash": "sha256:...",
  "build_os": "linux",
  "build_arch": "amd64"
}
```

The installer verifies:

- `build_os` and `build_arch` match the current platform
- `groot_version` is compatible with the current Groot build version
- `integration_spec_hash` matches the plugin's runtime `IntegrationSpec`

## Signing

Each package includes `signature.ed25519`.

The signature is verified against:

- `sha256(integration.so + manifest.json)`

Only packages signed by a configured trusted publisher key may be installed.

Trusted keys are loaded from `GROOT_INTEGRATION_TRUSTED_KEYS_PATH`.

Example:

```json
{
  "trusted_publishers": [
    {
      "name": "Groot Official",
      "public_key": "ed25519:BASE64..."
    }
  ]
}
```

The matched trusted key entry name becomes the package publisher identity shown in Groot.

## Installation

Build the CLI:

```sh
go build -o ./bin/groot ./cmd/groot
```

Install from a local file:

```sh
./bin/groot integration install ./customcrm-1.0.0.grootpkg
```

Install from a registry:

```sh
export GROOT_INTEGRATION_REGISTRY_URL=https://integrations.groot.dev/index.json
./bin/groot integration install customcrm
```

List and inspect installed integrations:

```sh
./bin/groot integration list
./bin/groot integration info customcrm
```

Remove a integration:

```sh
./bin/groot integration remove customcrm
```

## Storage Layout

By default Phase 30 uses:

- plugin dir: `integrations/plugins`
- trusted keys: `integrations/trusted_keys.json`
- installed metadata: `integrations/installed.json`
- package cache: `integrations/cache`

Installed integrations are loaded by the Phase 29 plugin loader on the next Groot startup.

## Registry Index

Registry installs use an optional index document:

```json
{
  "integrations": [
    {
      "name": "customcrm",
      "versions": [
        {
          "version": "1.0.0",
          "package_url": "https://integrations.groot.dev/customcrm-1.0.0.grootpkg",
          "checksum": "sha256:..."
        }
      ]
    }
  ]
}
```

Downloaded packages are checksum-verified before signature verification.
