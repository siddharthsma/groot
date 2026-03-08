# Example Provider Plugin

This example shows the minimum shape of an external Groot provider plugin.

It exports:

```go
var Provider provider.Provider = &EchoProvider{}
```

Build it with:

```sh
go build -buildmode=plugin -o example_echo_provider.so
```

Copy the resulting `.so` into the directory configured by `GROOT_PROVIDER_PLUGIN_DIR`.
