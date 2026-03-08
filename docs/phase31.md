
# Groot — Phase 31

## Goal

Set up the frontend workspace for Groot.

Phase 31 establishes the frontend foundation required for future UI phases including:

- dynamic pages
- interactive forms
- graph/workflow builders
- reusable UI components
- consistent theming

This phase ONLY sets up the frontend workspace, installs dependencies, and creates the directory structure.

No product UI is implemented yet.

---

# Scope

Phase 31 implements:

1. Frontend workspace creation
2. Core frontend dependency installation
3. Next.js App Router + TypeScript setup
4. Tailwind CSS configuration
5. shadcn/ui initialization
6. React Query setup
7. React Flow installation
8. Initial directory structure
9. Minimal application shell
10. Frontend build scripts
11. Documentation updates

---

# Frontend Stack

Core framework:

- Next.js
- React
- TypeScript

Styling and components:

- Tailwind CSS
- shadcn/ui

Forms and validation:

- react-hook-form
- zod

Server state:

- @tanstack/react-query

Local UI state:

- zustand

Tables:

- @tanstack/react-table

Graph/workflow UI:

- @xyflow/react
- dagre

Icons/utilities:

- lucide-react
- date-fns
- clsx
- tailwind-merge

---

# Workspace Structure

Create the frontend workspace:

```
ui/
```

Directory structure:

```
ui/
  app/
    layout.tsx
    page.tsx
    globals.css

  components/
    ui/
    layout/
    forms/
    graphs/
    tables/
    providers/
    agents/
    events/

  lib/
    api/
    query/
    utils/
    schemas/
    theme/

  hooks/
  types/
  public/
  styles/
  tests/
```

Rules:

- app/ uses Next.js App Router
- components/ui/ holds shadcn/ui generated components
- lib/api/ contains API helpers
- lib/query/ contains React Query client setup

---

# Package Initialization

Inside `ui/` initialize a Next.js TypeScript application.

Requirements:

- App Router enabled
- TypeScript enabled
- ESLint enabled
- Tailwind installed

Preferred package manager:

```
pnpm
```

---

# Runtime Dependencies

Install:

```
next
react
react-dom
@tanstack/react-query
@tanstack/react-table
@xyflow/react
dagre
react-hook-form
zod
zustand
lucide-react
date-fns
clsx
tailwind-merge
```

---

# Dev Dependencies

Install:

```
typescript
@types/node
@types/react
@types/react-dom
tailwindcss
postcss
autoprefixer
eslint
eslint-config-next
```

---

# shadcn/ui Setup

Initialize shadcn/ui.

Expected outcomes:

- components.json created
- Tailwind configuration updated
- base utility helper created

Generate the following base components:

- button
- card
- input
- textarea
- label
- dialog
- dropdown-menu
- sheet
- table
- badge
- tabs
- form
- select

---

# Query Client Setup

Create:

```
ui/lib/query/client.ts
ui/lib/query/provider.tsx
```

Requirements:

- single QueryClient instance
- provider mounted at app root

---

# API Client Setup

Create:

```
ui/lib/api/client.ts
ui/lib/api/types.ts
```

Environment variable:

```
NEXT_PUBLIC_GROOT_API_BASE_URL
```

Example:

```
NEXT_PUBLIC_GROOT_API_BASE_URL=http://localhost:8080
```

---

# Graph Foundation

Create:

```
ui/components/graphs/GraphCanvas.tsx
ui/components/graphs/types.ts
ui/components/graphs/layout.ts
```

Requirements:

- React Flow canvas renders
- Dagre layout helper included

No graph logic implemented.

---

# Form Foundation

Create:

```
ui/components/forms/FormField.tsx
ui/components/forms/FormSection.tsx
ui/components/forms/FormActions.tsx
```

Use:

- React Hook Form
- shadcn components

---

# Layout Foundation

Create minimal layout shell.

Files:

```
ui/components/layout/AppShell.tsx
ui/components/layout/Sidebar.tsx
ui/components/layout/Header.tsx
```

App page may display:

```
"Groot UI initialized"
```

---

# Initial Routes

Create placeholder pages:

```
/app/page.tsx
/app/providers/page.tsx
/app/events/page.tsx
/app/agents/page.tsx
```

These pages may contain placeholder content.

---

# Environment Files

Create:

```
ui/.env.example
```

Contents:

```
NEXT_PUBLIC_GROOT_API_BASE_URL=http://localhost:8080
```

---

# Scripts

Add scripts in `ui/package.json`:

```
dev
build
start
lint
typecheck
test
```

Rules:

- lint must succeed
- typecheck must succeed

---

# Verification

Verify:

1. dependencies install
2. dev server starts
3. build succeeds
4. lint passes
5. typecheck passes
6. routes render
7. React Flow canvas renders
8. Query client mounts
9. shadcn components render

---

# Documentation

Update:

```
README.md
AGENTS.md
docs/codebase_structure.md
```

Add sections describing:

- frontend workspace
- stack
- setup instructions
- local dev workflow

---

# Out of Scope

Phase 31 does NOT implement:

- provider marketplace UI
- agent studio
- workflow builder
- event graph explorer
- authentication UI
- RBAC UI

Only infrastructure is implemented.

---

# Completion Criteria

Phase 31 is complete when:

- `ui/` workspace exists
- Next.js App Router + TypeScript initialized
- Tailwind + shadcn/ui installed
- React Query installed and wired
- React Flow installed and rendering placeholder canvas
- directory structure created
- placeholder routes render
- build/lint/typecheck succeed
- documentation updated
