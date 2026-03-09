# Groot UI

This workspace contains the standalone Next.js frontend scaffold introduced in Phase 31.

## Commands

```sh
cp .env.example .env
pnpm install
pnpm dev
pnpm lint
pnpm typecheck
pnpm build
```

## Environment

```sh
NEXT_PUBLIC_GROOT_API_BASE_URL=http://localhost:8081
```

## Scope

Phase 31 sets up:

- Next.js App Router with TypeScript
- Tailwind CSS and shadcn/ui base components
- React Query integration wiring
- API client scaffolding
- graph, form, table, integration, event, and agent component directories

No product UI is implemented here yet. The routes and components are placeholders that establish the structure for later phases.
