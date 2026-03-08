
# Groot — Phase 29

## Goal

Introduce a **Provider Plugin System** that allows providers to be developed and distributed **outside the core Groot repository** while remaining safely integrated with the Provider Framework introduced in Phases 27–28.

Phase 29 allows:

- external developers to build providers independently
- operators to install additional providers
- Groot to load providers dynamically at startup

Providers remain **trusted binaries**, not arbitrary runtime code.

No WASM runtime.
No network plugin loading.

Plugins are **local filesystem modules** loaded at startup.

---

# Scope

Phase 29 implements:

1. Provider plugin loading system
2. External provider module format
3. Plugin discovery at startup
4. Plugin validation and safety checks
5. Provider registry integration with plugins
6. Plugin development SDK
7. Plugin packaging guidelines
8. Plugin integration tests

---

# Principles

Rules:

- plugin providers must implement the Phase 27 Provider interface
- plugin providers must expose a ProviderSpec
- plugin providers must be loaded only at startup
- plugins must be located in a configured directory
- plugin loading must fail fast on errors
- plugin loading must not allow runtime code injection

Providers must be deterministic and safe.

---

# Plugin Directory

Introduce configurable plugin directory.

Environment variable:

GROOT_PROVIDER_PLUGIN_DIR

Example:

/opt/groot/providers

If the directory does not exist:

- startup proceeds normally
- no plugins loaded

---

# Plugin File Format

Plugins must be compiled Go plugins.

Expected extension:

.so

Example files:

/opt/groot/providers/slackplus.so
/opt/groot/providers/customcrm.so

Each plugin must expose a symbol:

Provider

The symbol must implement the Provider interface defined in Phase 27.

---

# Plugin Loader

Create package:

internal/connectors/pluginloader

Add files:

loader.go
scan.go
validate.go

Responsibilities:

- scan plugin directory
- load plugin files
- resolve exported Provider symbol
- validate ProviderSpec
- register provider in Provider Registry

---

# Plugin Loading Process

Startup sequence must perform:

1. read plugin directory
2. find all `.so` files
3. load each plugin via Go plugin package
4. extract exported symbol `Provider`
5. verify symbol implements Provider interface
6. validate ProviderSpec
7. register provider in registry

---

# Plugin Validation

Validation must enforce:

1. ProviderSpec name unique
2. plugin provider name not equal to core provider unless override allowed
3. ProviderSpec valid under Phase 27 rules
4. schemas declared exist in registry
5. operation names valid
6. config fields valid

Startup must fail if plugin validation fails.

---

# Plugin Override Rules

Plugins must not silently override core providers.

Default rule:

- plugin provider name conflict causes startup failure

Optional future extension:

GROOT_ALLOW_PROVIDER_OVERRIDE=true

If enabled:

- plugin may replace core provider implementation

Phase 29 should **not implement override logic yet**, only detect conflicts.

---

# Plugin SDK

Create SDK module to help plugin authors.

Location:

sdk/provider

Contents:

provider.go
helpers.go
types.go

SDK must expose:

- Provider interface
- ProviderSpec types
- config validation helpers
- schema helpers
- operation helpers

Plugin authors should import SDK instead of internal packages.

---

# Example Plugin

Add example plugin repository in:

examples/provider-plugin

Example provider:

example_echo_provider

Capabilities:

- one operation `echo`
- no inbound routes
- simple config

Example plugin must demonstrate:

- ProviderSpec
- operation implementation
- plugin export symbol

Example code snippet:

var Provider provider.Provider = &EchoProvider{}

---

# Plugin Packaging

Define packaging format.

Recommended structure:

myprovider/
  go.mod
  provider.go
  config.go
  operations.go
  schemas.go

Build command:

go build -buildmode=plugin -o myprovider.so

Resulting `.so` file copied into plugin directory.

---

# Plugin Discovery API Extension

Extend Phase 28 discovery endpoints to include plugin providers.

Provider detail responses must include field:

source

Values:

core
plugin

Example:

{
  "name": "customcrm",
  "source": "plugin"
}

---

# Plugin Error Handling

Plugin loading errors must be explicit.

Possible errors:

- plugin load failure
- symbol not found
- symbol wrong type
- ProviderSpec invalid
- name conflict

Startup logs must clearly show plugin failure reason.

Startup must stop if plugin load fails.

---

# Plugin Isolation

Plugins must not access internal packages.

Rules:

- plugin must depend only on public SDK
- plugin must not import internal/*
- plugin must compile independently

Enforcement:

- build docs
- plugin validation warnings

---

# Plugin Tests

Add integration tests:

tests/integration/phase29_plugin_loading_test.go

Tests must verify:

1. plugin loads successfully
2. plugin provider appears in provider catalog
3. plugin operations callable
4. plugin config validation works
5. duplicate provider names rejected
6. invalid plugin symbol rejected

---

# Documentation

Add plugin documentation.

Location:

docs/providers/plugins.md

Sections:

- Plugin overview
- Plugin architecture
- Plugin build instructions
- Provider SDK usage
- Plugin deployment
- Plugin troubleshooting

---

# Build Integration

Plugins must not be required for normal builds.

Rules:

- plugin directory optional
- plugin loader activated only if directory exists

Ensure:

go build ./...
go test ./...
make checkpoint

succeed without plugins.

---

# Security Considerations

Plugins are trusted code.

Rules:

- plugins loaded only from local filesystem
- no network plugin loading
- no runtime plugin installation

Operators must control plugin directory.

---

# Out of Scope

Phase 29 must not include:

- remote plugin registry
- marketplace
- plugin version negotiation
- sandboxing
- WASM execution
- dynamic runtime installation
- plugin hot reload

Plugins load **only at startup**.

---

# Phase 29 Completion Criteria

All conditions must be met:

- plugin loader implemented
- plugin directory supported
- plugin providers load successfully
- plugin providers appear in provider catalog
- plugin validation prevents unsafe providers
- SDK available for plugin developers
- example plugin created
- plugin documentation added
- integration tests pass
- checkpoint pipeline passes
