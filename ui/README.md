# Groot UI

This workspace contains the standalone Next.js frontend app for Groot.

Phase 38 establishes the core in-app shell, and Phase 38B refines its
personality:

- the cosmic dark token system
- the green-primary and cyan-accent visual direction
- the grouped tenant navigation
- the shared top bar and page shell
- placeholder pages for the main product sections

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

The workspace currently provides:

- Next.js App Router with TypeScript
- Tailwind CSS and shadcn/ui base components
- React Query integration wiring
- API client scaffolding
- a cosmic dark theme token source of truth in `app/globals.css`
- grouped tenant navigation for:
  - Overview
  - Integrations
  - Connections
  - Workflows
  - Agents
  - Event Stream
  - Runs
- reusable shell components under `components/layout/`

No product page content is implemented yet. The current routes are intentionally placeholder-only and exist to prove the design system and shell foundation before real data-driven screens are added.

## Theme Notes

The shell is dark-only in this phase. Semantic color tokens, motion tokens,
radius tokens, workflow colors, and chart colors are defined in
`app/globals.css`.

The current baseline uses:

- `Space Grotesk` for the UI sans
- `JetBrains Mono` for technical surfaces
- a deep navy to dark teal background
- green as the primary signal color
- cyan and violet as secondary accents

Future pages should:

- use semantic token classes instead of hardcoded colors
- render inside `AppShell`
- use `PageHeader` plus `PlaceholderPanel`-style surfaces as the baseline
