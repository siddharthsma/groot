# Provider Plugins

Phase 29 adds support for trusted provider plugins loaded from the local filesystem at startup.

## How It Works

- Groot reads `GROOT_PROVIDER_PLUGIN_DIR`
- every `.so` file in that directory is opened during startup
- each plugin must export `var Provider provider.Provider`
- Groot validates the provider spec
- plugin-owned schemas are registered in the schema registry
- the plugin is added to the normal provider registry

After loading, plugin providers behave like built-in providers:

- they appear in `GET /providers`
- they can be used by `/connector-instances`
- they execute through the existing Temporal delivery path

## Plugin Contract

Plugins are compiled Go plugins.

They must:

- be built with `-buildmode=plugin`
- export a variable named `Provider`
- use the public SDK in `sdk/provider`
- avoid importing `internal/*`

## SDK

The plugin author SDK lives in the separate module:

- `sdk/provider`

Use it from an external plugin module instead of importing Groot internals directly.

## Example

An example plugin lives in:

- `examples/provider-plugin`

It implements `example_echo_provider`, a minimal provider with one `echo` operation.

Build it with:

```sh
cd examples/provider-plugin
go build -buildmode=plugin -o example_echo_provider.so .
```

Then place the resulting `.so` file in the directory configured by `GROOT_PROVIDER_PLUGIN_DIR`.

## Failure Behavior

Plugin loading is fail-fast.

Startup fails if:

- a plugin does not export `Provider`
- the exported symbol has the wrong type
- the provider name conflicts with an existing provider
- the provider spec is invalid
- plugin-owned schemas cannot be registered or do not match the registry
