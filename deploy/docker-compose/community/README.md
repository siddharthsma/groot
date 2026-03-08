# Community Docker Compose

This directory packages Groot Community Edition as a single-tenant Docker
Compose deployment.

This bundle is intended to be used with a Community build, where
`BuildEdition=community` is embedded into the binary or image at build time.
Changing `.env` does not convert this deployment into Cloud or Internal mode.

## Quickstart

```sh
cp .env.example .env
docker compose up --build
```

Services exposed by default:

- API: `http://localhost:8080`
- Agent runtime stub: `http://localhost:8090`
- Temporal UI: `http://localhost:8233`

The bundled `groot-agent-runtime` service is a stub that returns a deterministic
failure for `/sessions/run`. Replace it with a real runtime service if you want
`llm.agent` executions to succeed in Community deployments.
