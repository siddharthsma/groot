# Example Integration Plugin

This example shows the minimum shape of an external Groot integration plugin.

It exports:

```go
var Integration integration.Integration = &EchoIntegration{}
```

Build it with:

```sh
go build -buildmode=plugin -o example_echo_integration.so
```

Copy the resulting `.so` into the directory configured by `GROOT_INTEGRATION_PLUGIN_DIR`.
