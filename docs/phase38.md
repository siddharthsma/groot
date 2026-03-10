# Groot — Phase 38

## Goal

Set up the **core frontend shell** for Groot with:

1. the **cosmic dark design system**
2. the **top-level tenant navigation**
3. placeholder page shells only

Phase 38 is the phase where Groot’s UI gets its **visual identity and structural shell**.

This phase must establish:

- theme tokens
- Tailwind + shadcn theme integration
- shared layout shell
- sidebar navigation
- top bar
- tenant-scoped page routing
- placeholder pages for the main sections

This phase must **not** yet build actual page content, workflow builder functionality, run visualization, or marketplace detail screens.

---

# Scope

Phase 38 implements:

1. Cosmic dark design theme setup
2. Tailwind and shadcn token integration
3. Global typography, spacing, radius, shadow, and motion tokens
4. App shell layout
5. Core tenant navigation
6. Top bar / shell header
7. Placeholder pages for top-level sections
8. Shared navigation and layout components
9. Basic responsive behavior for the shell
10. Documentation updates

---

# Principles

Rules:

- Phase 38 is **shell + theme only**
- no business content beyond placeholders
- no workflow builder interactions
- no real dashboard widgets
- no real integrations marketplace content
- no workflow/agent forms
- no event stream tables
- no run viewer
- no page-specific data fetching beyond what is strictly needed to render the shell

The purpose is to establish a stable **design system and navigation foundation**.

---

# Design Direction

The UI theme must follow the agreed design language:

- **cosmic dark base**
- **calm SaaS readability**
- **violet primary accent**
- **cyan secondary accent**
- **soft glass / elevated panels**
- **minimal glow**
- **80% calm, 20% wow**

The shell should feel:

> powerful, modern, technical, and premium — without becoming visually noisy.

---

# Theme System Setup

Phase 38 must implement the design system tokens in the frontend workspace.

This includes:

- CSS custom properties in `globals.css`
- Tailwind theme extension
- shadcn/ui variable alignment
- product-specific semantic tokens

---

## Core Semantic Tokens

Implement and wire these semantic tokens:

### Base
- `background`
- `foreground`
- `card`
- `card-foreground`
- `popover`
- `popover-foreground`
- `border`
- `input`
- `ring`

### Brand
- `primary`
- `primary-foreground`
- `secondary`
- `secondary-foreground`
- `accent`
- `accent-foreground`

### State
- `success`
- `warning`
- `destructive`
- `info`

### Surface
- `surface-1`
- `surface-2`
- `surface-3`

### Workflow-specific
- `workflow-trigger`
- `workflow-action`
- `workflow-agent`
- `workflow-wait`
- `workflow-end`

### Charts
- `chart-1`
- `chart-2`
- `chart-3`
- `chart-4`
- `chart-5`

---

## Required Color Direction

Implement the agreed cosmic dark palette approximately as:

- deep navy / space background
- dark elevated panels
- violet primary accent
- cyan accent
- soft blue info
- amber wait state
- green success
- rose destructive

The exact HSL/CSS variable values should be taken from the agreed token set and wired into shadcn/Tailwind.

---

## Radius Tokens

Set and standardize radius usage:

- small rounded controls
- medium inputs/buttons
- large cards/panels
- extra-large shell containers

The shell should use a visibly premium modern rounding system.

---

## Shadow Tokens

Implement:

- subtle dark elevation shadows
- stronger large-panel shadows
- restrained glow shadows for:
  - primary CTAs
  - active nav items
  - selected builder surfaces later

Do **not** overuse glows on all components.

---

## Motion Tokens

Set up standardized transition timing/easing for:

- hover states
- focus states
- sidebar interactions
- route shell transitions if minimal

Do not add heavy animation in Phase 38.

---

# Tailwind and shadcn Integration

Phase 38 must wire the design system cleanly into Tailwind and shadcn.

Required outcomes:

- `globals.css` contains the CSS variable theme tokens
- `tailwind.config.*` is extended to expose semantic color groups
- shadcn components resolve to the new token set automatically
- no component should rely on hardcoded one-off colors where semantic tokens exist

This phase must leave the app ready for consistent future component styling.

---

# Typography System

Phase 38 must establish typography defaults.

## Required setup

- primary UI font configured globally
- optional secondary heading font only if already practical
- monospace font configured for future code/payload viewers

Recommended usage patterns must be established for:

- page titles
- section titles
- nav labels
- body text
- muted support text
- badges/chips
- future monospace data areas

No page-specific content is needed, but the typographic system must be visibly in place.

---

# Core Layout Shell

Phase 38 must add the core application layout.

## Required shell regions

1. **Sidebar**
2. **Top bar**
3. **Main content container**

The shell must be reusable across all main tenant pages.

---

## Sidebar Requirements

The sidebar must contain the final agreed top-level tenant navigation.

### Navigation groups and items

#### Overview
- Overview

#### Source
- Integrations
- Connections

#### Build
- Workflows
- Agents

#### Monitor
- Event Stream
- Runs

### Rules

- navigation should be grouped and visually labeled
- active route styling must exist
- hover/active states must match the design theme
- icons must be included for each top-level item
- labels must be clear and stable
- nav terminology must use current canonical product language:
  - Integrations
  - Connections
  - Workflows
  - Agents
  - Event Stream
  - Runs

---

## Tenant Context in Shell

The shell should visibly communicate tenant context.

Add a tenant indicator area in the sidebar or top bar, such as:

- tenant name
- optional environment/status chip

This may use placeholder tenant information in Phase 38, but the shell must have a dedicated place for tenant identity.

No real tenant switching logic is required yet.

---

## Top Bar Requirements

The top bar must include:

- current page title region
- optional global search / command bar placeholder
- right-side action area placeholder
- visual alignment with the cosmic dark shell style

Allowed placeholder elements:

- search field shell
- command shortcut chip
- action button placeholder

No real search behavior required yet.

---

# Navigation Components

Create reusable layout/navigation components.

Suggested components:

- `AppShell`
- `SidebarNav`
- `SidebarNavSection`
- `SidebarNavItem`
- `TopBar`
- `PageHeader`
- `TenantBadge` or equivalent
- `ShellContainer`

Rules:

- shell components must be layout-only
- no page-specific content logic
- styling must come from the shared design system

---

# Placeholder Pages

Phase 38 must add placeholder pages for the top-level sections only.

Required routes/pages:

- Overview
- Integrations
- Connections
- Workflows
- Agents
- Event Stream
- Runs

Each page may render only:

- page title
- short placeholder text such as “TBC”
- optional page shell card/panel to demonstrate layout consistency

No real content, forms, lists, or tables yet.

---

# Placeholder Page Rules

Each placeholder page must:

- render inside the shared shell
- use consistent page header styling
- use a consistent card/panel placeholder surface
- confirm that the nav highlights correctly
- confirm route-level shell behavior works

The placeholder pages are purely structural.

---

# Responsive Behavior

Phase 38 must implement basic responsive shell behavior.

Minimum requirements:

- sidebar and top bar remain usable on narrower widths
- layout does not break on laptop-sized screens
- mobile can be minimal/deferred, but the shell should degrade cleanly
- no advanced mobile UX required yet

The goal is not a fully polished responsive product, only a solid shell foundation.

---

# Component Styling Requirements

Phase 38 must set the baseline styling for these shared UI primitives:

- buttons
- cards
- panels
- inputs
- badges/chips
- navigation items
- separators/dividers
- placeholders/empty states

These should all visibly reflect the Groot design system.

No need to add many new custom components beyond what the shell needs.

---

# Iconography

Use a consistent icon set already present in the frontend stack.

Rules:

- one icon style only
- icons for all nav items
- icon size, color, and active-state behavior standardized

No custom illustration work required in this phase.

---

# Empty / Placeholder State Style

Since pages have no content yet, Phase 38 should define a consistent “TBC” or placeholder treatment.

This should feel deliberate, not broken.

Each placeholder should use:

- a styled panel/card
- title
- concise placeholder text
- theme-consistent visual treatment

This becomes the standard temporary content treatment during UI rollout.

---

# Accessibility Baseline

Phase 38 must include baseline accessibility checks for the shell:

- sufficient text contrast in dark mode
- visible focus states
- keyboard navigable nav items
- semantic landmarks where practical
- screen-reader-friendly labels for major nav controls

No advanced accessibility phase is required yet, but the shell must not start from an inaccessible baseline.

---

# Files / Areas Expected to Change

Phase 38 will likely touch:

- `ui/app/layout.*`
- `ui/app/page.*`
- top-level route files for the placeholder pages
- `ui/components/layout/*`
- `ui/components/navigation/*` if separated
- `ui/components/ui/*` where small style variants are needed
- `ui/lib/theme/*`
- `ui/globals.css`
- `ui/tailwind.config.*`
- shadcn configuration files if necessary

You do not need to force this exact structure, but the shell/theme concerns must be clearly separated from future page content.

---

# Scripts / Verification

Phase 38 must ensure the frontend still supports:

- dev
- build
- lint
- typecheck

The shell and placeholder pages must build cleanly.

---

# Visual Verification Checklist

Phase 38 should be considered visually correct only if all of the following are true:

1. the app opens with the cosmic dark shell active
2. sidebar navigation is grouped correctly
3. top bar visually matches the shell
4. active page highlighting works
5. placeholder pages feel consistent
6. primary/accent colors are visible but restrained
7. panels/cards use the shared token system
8. overall UI feels calm, premium, and modern

---

# Documentation

Update relevant frontend documentation to describe:

- the Groot UI design direction
- the implemented theme/token system
- core navigation structure
- how future pages should use the shell and theme tokens

Recommended docs to update:

- frontend README / setup notes
- codebase structure documentation
- any UI/design notes file if present

---

# Out of Scope

Phase 38 must not include:

- workflow builder interactions
- agent builder interactions
- dashboard widgets
- integrations marketplace content
- connection forms
- workflow forms
- agent forms
- event stream table
- runs table
- live data fetching for page content
- graph canvas work beyond future planning

This phase is strictly:

> **theme + shell + top-level nav + placeholder pages**

---

# Recommended Follow-On

After Phase 38, the next natural UI phases are:

## Phase 39
Workflow Builder UI

## Phase 40
Workflow Run Visualization UI

## Phase 41
Integrations and Connections pages

Phase 38 should not bleed into those.

---

# Phase 38 Completion Criteria

All conditions must be met:

- cosmic dark design system is implemented
- Tailwind + shadcn tokens are wired to the Groot theme
- typography, radius, shadow, spacing, and motion foundations are in place
- reusable shell/layout components exist
- top-level tenant navigation is implemented exactly as agreed
- tenant context area exists in the shell
- top bar exists and is visually integrated
- placeholder pages exist for:
  - Overview
  - Integrations
  - Connections
  - Workflows
  - Agents
  - Event Stream
  - Runs
- active nav highlighting works
- frontend build/lint/typecheck succeed
- documentation is updated