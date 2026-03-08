# Provider Packages

Phase 30 introduces signed provider packages for installing external providers outside the core repository.

## Package Format

Provider packages use the `.grootpkg` extension.

They are tar archives with this layout:

```text
provider/
  provider.so
  manifest.json
  signature.ed25519
```

## Manifest

`manifest.json` describes the package:

```json
{
  "name": "customcrm",
  "version": "1.0.0",
  "description": "Custom CRM provider",
  "author": "Example Corp",
  "groot_version": ">=1.0.0",
  "provider_spec_hash": "sha256:...",
  "build_os": "linux",
  "build_arch": "amd64"
}
```

The installer verifies:

- `build_os` and `build_arch` match the current platform
- `groot_version` is compatible with the current Groot build version
- `provider_spec_hash` matches the plugin's runtime `ProviderSpec`

## Signing

Each package includes `signature.ed25519`.

The signature is verified against:

- `sha256(provider.so + manifest.json)`

Only packages signed by a configured trusted publisher key may be installed.

Trusted keys are loaded from `GROOT_PROVIDER_TRUSTED_KEYS_PATH`.

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
./bin/groot provider install ./customcrm-1.0.0.grootpkg
```

Install from a registry:

```sh
export GROOT_PROVIDER_REGISTRY_URL=https://providers.groot.dev/index.json
./bin/groot provider install customcrm
```

List and inspect installed providers:

```sh
./bin/groot provider list
./bin/groot provider info customcrm
```

Remove a provider:

```sh
./bin/groot provider remove customcrm
```

## Storage Layout

By default Phase 30 uses:

- plugin dir: `providers/plugins`
- trusted keys: `providers/trusted_keys.json`
- installed metadata: `providers/installed.json`
- package cache: `providers/cache`

Installed providers are loaded by the Phase 29 plugin loader on the next Groot startup.

## Registry Index

Registry installs use an optional index document:

```json
{
  "providers": [
    {
      "name": "customcrm",
      "versions": [
        {
          "version": "1.0.0",
          "package_url": "https://providers.groot.dev/customcrm-1.0.0.grootpkg",
          "checksum": "sha256:..."
        }
      ]
    }
  ]
}
```

Downloaded packages are checksum-verified before signature verification.
