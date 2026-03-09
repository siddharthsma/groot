# Integration Plugins

Phase 29 adds support for trusted integration plugins loaded from the local filesystem at startup.

## How It Works

- Groot reads `GROOT_INTEGRATION_PLUGIN_DIR`
- every `.so` file in that directory is opened during startup
- each plugin must export `var Integration integration.Integration`
- Groot validates the integration spec
- plugin-owned schemas are registered in the schema registry
- the plugin is added to the normal integration registry

After loading, plugin integrations behave like built-in integrations:

- they appear in `GET /integrations`
- they can be used by `/connections`
- they execute through the existing Temporal delivery path

## Plugin Contract

Plugins are compiled Go plugins.

They must:

- be built with `-buildmode=plugin`
- export a variable named `Integration`
- use the public SDK in `sdk/integration`
- avoid importing `internal/*`

## SDK

The plugin author SDK lives in the separate module:

- `sdk/integration`

Use it from an external plugin module instead of importing Groot internals directly.

## Example

An example plugin lives in:

- `examples/integration-plugin`

It implements `example_echo_integration`, a minimal integration with one `echo` operation.

Build it with:

```sh
cd examples/integration-plugin
go build -buildmode=plugin -o example_echo_integration.so .
```

Then place the resulting `.so` file in the directory configured by `GROOT_INTEGRATION_PLUGIN_DIR`.

## Failure Behavior

Plugin loading is fail-fast.

Startup fails if:

- a plugin does not export `Integration`
- the exported symbol has the wrong type
- the integration name conflicts with an existing integration
- the integration spec is invalid
- plugin-owned schemas cannot be registered or do not match the registry
