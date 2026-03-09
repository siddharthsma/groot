
# Groot — Phase 29

## Goal

Introduce a **Integration Plugin System** that allows integrations to be developed and distributed **outside the core Groot repository** while remaining safely integrated with the Integration Framework introduced in Phases 27–28.

Phase 29 allows:

- external developers to build integrations independently
- operators to install additional integrations
- Groot to load integrations dynamically at startup

Integrations remain **trusted binaries**, not arbitrary runtime code.

No WASM runtime.
No network plugin loading.

Plugins are **local filesystem modules** loaded at startup.

---

# Scope

Phase 29 implements:

1. Integration plugin loading system
2. External integration module format
3. Plugin discovery at startup
4. Plugin validation and safety checks
5. Integration registry integration with plugins
6. Plugin development SDK
7. Plugin packaging guidelines
8. Plugin integration tests

---

# Principles

Rules:

- plugin integrations must implement the Phase 27 Integration interface
- plugin integrations must expose a IntegrationSpec
- plugin integrations must be loaded only at startup
- plugins must be located in a configured directory
- plugin loading must fail fast on errors
- plugin loading must not allow runtime code injection

Integrations must be deterministic and safe.

---

# Plugin Directory

Introduce configurable plugin directory.

Environment variable:

GROOT_INTEGRATION_PLUGIN_DIR

Example:

/opt/groot/integrations

If the directory does not exist:

- startup proceeds normally
- no plugins loaded

---

# Plugin File Format

Plugins must be compiled Go plugins.

Expected extension:

.so

Example files:

/opt/groot/integrations/slackplus.so
/opt/groot/integrations/customcrm.so

Each plugin must expose a symbol:

Integration

The symbol must implement the Integration interface defined in Phase 27.

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
- resolve exported Integration symbol
- validate IntegrationSpec
- register integration in Integration Registry

---

# Plugin Loading Process

Startup sequence must perform:

1. read plugin directory
2. find all `.so` files
3. load each plugin via Go plugin package
4. extract exported symbol `Integration`
5. verify symbol implements Integration interface
6. validate IntegrationSpec
7. register integration in registry

---

# Plugin Validation

Validation must enforce:

1. IntegrationSpec name unique
2. plugin integration name not equal to core integration unless override allowed
3. IntegrationSpec valid under Phase 27 rules
4. schemas declared exist in registry
5. operation names valid
6. config fields valid

Startup must fail if plugin validation fails.

---

# Plugin Override Rules

Plugins must not silently override core integrations.

Default rule:

- plugin integration name conflict causes startup failure

Optional future extension:

GROOT_ALLOW_PROVIDER_OVERRIDE=true

If enabled:

- plugin may replace core integration implementation

Phase 29 should **not implement override logic yet**, only detect conflicts.

---

# Plugin SDK

Create SDK module to help plugin authors.

Location:

sdk/integration

Contents:

integration.go
helpers.go
types.go

SDK must expose:

- Integration interface
- IntegrationSpec types
- config validation helpers
- schema helpers
- operation helpers

Plugin authors should import SDK instead of internal packages.

---

# Example Plugin

Add example plugin repository in:

examples/integration-plugin

Example integration:

example_echo_integration

Capabilities:

- one operation `echo`
- no inbound routes
- simple config

Example plugin must demonstrate:

- IntegrationSpec
- operation implementation
- plugin export symbol

Example code snippet:

var Integration integration.Integration = &EchoIntegration{}

---

# Plugin Packaging

Define packaging format.

Recommended structure:

myintegration/
  go.mod
  integration.go
  config.go
  operations.go
  schemas.go

Build command:

go build -buildmode=plugin -o myintegration.so

Resulting `.so` file copied into plugin directory.

---

# Plugin Discovery API Extension

Extend Phase 28 discovery endpoints to include plugin integrations.

Integration detail responses must include field:

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
- IntegrationSpec invalid
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
2. plugin integration appears in integration catalog
3. plugin operations callable
4. plugin config validation works
5. duplicate integration names rejected
6. invalid plugin symbol rejected

---

# Documentation

Add plugin documentation.

Location:

docs/integrations/plugins.md

Sections:

- Plugin overview
- Plugin architecture
- Plugin build instructions
- Integration SDK usage
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
- plugin integrations load successfully
- plugin integrations appear in integration catalog
- plugin validation prevents unsafe integrations
- SDK available for plugin developers
- example plugin created
- plugin documentation added
- integration tests pass
- checkpoint pipeline passes
