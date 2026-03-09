
# Groot — Phase 30

## Goal

Introduce a **Signed Integration Package System and Integration Registry** so integrations can be safely distributed and installed outside the core Groot repository.

Phase 30 enables:

- integrations distributed as signed packages
- operator installation of integrations from a registry
- verification of integration integrity and authenticity
- integration lifecycle management

Phase 30 builds on:

- Phase 27 Integration Framework
- Phase 28 Integration Catalog
- Phase 29 Integration Plugin System

Integrations remain **trusted binaries loaded locally**.

No automatic remote code execution.

---

# Scope

Phase 30 implements:

1. Integration package format
2. Integration package signature verification
3. Integration registry index format
4. Integration installation CLI
5. Integration package cache
6. Integration metadata verification
7. Integration version management
8. Integration lifecycle commands
9. Integration registry client
10. Integration tests for integration installation and verification

---

# Principles

Rules:

- integrations must be signed before installation
- integration metadata must be verifiable
- Groot must never load unsigned integrations
- integration installation must be explicit
- integration registry must be optional

Integration execution model remains identical to Phase 29 plugins.

---

# Integration Package Format

Introduce integration package extension:

.grootpkg

Example:

slackplus-1.0.0.grootpkg
customcrm-0.2.1.grootpkg

A integration package is a **tar archive** with the following structure:

integration/
  integration.so
  manifest.json
  signature.ed25519

---

# Integration Manifest

Each integration package must include:

manifest.json

Example:

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

---

# Integration Signature

Each integration package must include:

signature.ed25519

Signature must be created using the integration publisher private key.

Signed content:

sha256(integration.so + manifest.json)

Groot verifies signature using known public keys.

---

# Trusted Publisher Keys

Introduce publisher key configuration.

Config file:

integrations/trusted_keys.json

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

# Integration Registry Index

Introduce registry index format.

Example registry URL:

https://integrations.groot.dev/index.json

Index structure:

{
  "integrations": [
    {
      "name": "slackplus",
      "versions": [
        {
          "version": "1.0.0",
          "package_url": "https://integrations.groot.dev/slackplus-1.0.0.grootpkg",
          "checksum": "sha256:..."
        }
      ]
    }
  ]
}

Registry is optional. Operators may install packages manually.

---

# Integration Installer

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

# Integration CLI Commands

Extend Groot CLI.

Commands:

groot integration install <name>
groot integration install <file>
groot integration remove <name>
groot integration list
groot integration info <name>

Examples:

groot integration install slackplus
groot integration install ./customcrm-1.0.0.grootpkg

---

# Installation Location

Installed integration binaries stored in:

integrations/plugins

Example:

integrations/plugins/slackplus.so

Integration loader from Phase 29 loads plugins from this directory.

---

# Version Management

Only one version of a integration may be active.

Installing a new version must:

1. replace existing binary
2. restart Groot required

Version recorded in:

integrations/installed.json

Example:

{
  "integrations": [
    {
      "name": "slackplus",
      "version": "1.0.0"
    }
  ]
}

---

# Integration Registry Client

Create package:

internal/connectors/registryclient

Responsibilities:

- fetch registry index
- search integrations
- resolve latest compatible version
- download integration package

---

# Compatibility Checks

Installer must verify:

1. integration supports current OS/arch
2. integration compatible with Groot version
3. IntegrationSpec hash matches manifest
4. plugin loads successfully

Installation must fail if checks fail.

---

# Integration Catalog Integration

Phase 28 catalog must include additional metadata.

Integration detail response must include:

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

tests/integration/phase30_integration_installation_test.go

Tests must verify:

1. integration package installs correctly
2. invalid signature rejected
3. incompatible version rejected
4. integration loads after installation
5. integration removal works
6. catalog reflects installed integration

---

# Documentation

Add documentation:

docs/integrations/packages.md

Sections:

- Integration Package Format
- Signing Integrations
- Installing Integrations
- Registry Configuration
- Integration Lifecycle

---

# Security Model

Rules:

- integrations must be signed
- integrations must match trusted publisher keys
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

- auto-updating integrations
- remote execution
- sandboxing
- WASM integrations
- integration marketplace UI

---

# Phase 30 Completion Criteria

All conditions must be met:

- integration package format implemented
- integration signature verification implemented
- integration installer implemented
- integration CLI commands implemented
- integration registry client implemented
- integration catalog reflects installed integrations
- integration installation tests pass
- documentation complete
- checkpoint pipeline passes
