
# Groot — Phase 30

## Goal

Introduce a **Signed Provider Package System and Provider Registry** so providers can be safely distributed and installed outside the core Groot repository.

Phase 30 enables:

- providers distributed as signed packages
- operator installation of providers from a registry
- verification of provider integrity and authenticity
- provider lifecycle management

Phase 30 builds on:

- Phase 27 Provider Framework
- Phase 28 Provider Catalog
- Phase 29 Provider Plugin System

Providers remain **trusted binaries loaded locally**.

No automatic remote code execution.

---

# Scope

Phase 30 implements:

1. Provider package format
2. Provider package signature verification
3. Provider registry index format
4. Provider installation CLI
5. Provider package cache
6. Provider metadata verification
7. Provider version management
8. Provider lifecycle commands
9. Provider registry client
10. Integration tests for provider installation and verification

---

# Principles

Rules:

- providers must be signed before installation
- provider metadata must be verifiable
- Groot must never load unsigned providers
- provider installation must be explicit
- provider registry must be optional

Provider execution model remains identical to Phase 29 plugins.

---

# Provider Package Format

Introduce provider package extension:

.grootpkg

Example:

slackplus-1.0.0.grootpkg
customcrm-0.2.1.grootpkg

A provider package is a **tar archive** with the following structure:

provider/
  provider.so
  manifest.json
  signature.ed25519

---

# Provider Manifest

Each provider package must include:

manifest.json

Example:

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

---

# Provider Signature

Each provider package must include:

signature.ed25519

Signature must be created using the provider publisher private key.

Signed content:

sha256(provider.so + manifest.json)

Groot verifies signature using known public keys.

---

# Trusted Publisher Keys

Introduce publisher key configuration.

Config file:

providers/trusted_keys.json

Example:

{
  "trusted_publishers": [
    {
      "name": "Groot Official",
      "public_key": "ed25519:..."
    }
  ]
}

Only packages signed by trusted publishers may be installed.

---

# Provider Registry Index

Introduce registry index format.

Example registry URL:

https://providers.groot.dev/index.json

Index structure:

{
  "providers": [
    {
      "name": "slackplus",
      "versions": [
        {
          "version": "1.0.0",
          "package_url": "https://providers.groot.dev/slackplus-1.0.0.grootpkg",
          "checksum": "sha256:..."
        }
      ]
    }
  ]
}

Registry is optional. Operators may install packages manually.

---

# Provider Installer

Create package:

internal/connectors/installer

Files:

installer.go
download.go
verify.go
extract.go

Responsibilities:

- download package
- verify checksum
- verify signature
- extract plugin binary
- place plugin into plugin directory

---

# Provider CLI Commands

Extend Groot CLI.

Commands:

groot provider install <name>
groot provider install <file>
groot provider remove <name>
groot provider list
groot provider info <name>

Examples:

groot provider install slackplus
groot provider install ./customcrm-1.0.0.grootpkg

---

# Installation Location

Installed provider binaries stored in:

providers/plugins

Example:

providers/plugins/slackplus.so

Provider loader from Phase 29 loads plugins from this directory.

---

# Version Management

Only one version of a provider may be active.

Installing a new version must:

1. replace existing binary
2. restart Groot required

Version recorded in:

providers/installed.json

Example:

{
  "providers": [
    {
      "name": "slackplus",
      "version": "1.0.0"
    }
  ]
}

---

# Provider Registry Client

Create package:

internal/connectors/registryclient

Responsibilities:

- fetch registry index
- search providers
- resolve latest compatible version
- download provider package

---

# Compatibility Checks

Installer must verify:

1. provider supports current OS/arch
2. provider compatible with Groot version
3. ProviderSpec hash matches manifest
4. plugin loads successfully

Installation must fail if checks fail.

---

# Provider Catalog Integration

Phase 28 catalog must include additional metadata.

Provider detail response must include:

source
version
publisher

Example:

{
  "name": "customcrm",
  "source": "plugin",
  "version": "1.0.0",
  "publisher": "Example Corp"
}

---

# Tests

Add integration tests:

tests/integration/phase30_provider_installation_test.go

Tests must verify:

1. provider package installs correctly
2. invalid signature rejected
3. incompatible version rejected
4. provider loads after installation
5. provider removal works
6. catalog reflects installed provider

---

# Documentation

Add documentation:

docs/providers/packages.md

Sections:

- Provider Package Format
- Signing Providers
- Installing Providers
- Registry Configuration
- Provider Lifecycle

---

# Security Model

Rules:

- providers must be signed
- providers must match trusted publisher keys
- registry downloads must verify checksums
- plugins remain loaded only at startup

---

# Verification

Phase 30 must pass:

go build ./...
go test ./...
go vet ./...
make checkpoint

---

# Out of Scope

Phase 30 must not include:

- auto-updating providers
- remote execution
- sandboxing
- WASM providers
- provider marketplace UI

---

# Phase 30 Completion Criteria

All conditions must be met:

- provider package format implemented
- provider signature verification implemented
- provider installer implemented
- provider CLI commands implemented
- provider registry client implemented
- provider catalog reflects installed providers
- provider installation tests pass
- documentation complete
- checkpoint pipeline passes
