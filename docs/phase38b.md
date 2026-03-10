# Groot --- Phase 38B

## Theme Personality Adjustment (Groot Identity)

Goal: Adjust the completed Phase 38 UI shell to reflect Groot's brand
personality --- a living automation system where signals flow,
connections form, and workflows grow.

Scope: - Update theme tokens - Adjust accent/glow usage - Modify sidebar
and top bar styling - Update placeholder microcopy - Add workflow color
tokens for future phases - Subtle background gradient

No new functionality or pages are introduced in this phase.

Core Palette: background: #07111f surface-1: #0f1b31 surface-2: #13233d
foreground: #edf3ff muted-foreground: #8da0c7 border: #22314d input:
#1a2740

Brand Colors: primary: #34d399 primary-foreground: #022c22

Accent: accent: #22d3ee accent-foreground: #04131a

Agent Color: agent: #8b5cf6

State Colors: success: #10b981 warning: #f59e0b destructive: #fb7185
info: #60a5fa

Workflow Tokens: workflow-trigger: #22d3ee workflow-action: #60a5fa
workflow-agent: #8b5cf6 workflow-wait: #f59e0b workflow-end: #10b981

Glow Usage: Allow glow only on: - active sidebar item - primary button -
focused input - future selected workflow node

Example glow: 0 0 0 1px rgba(52,211,153,0.25) 0 10px 30px
rgba(52,211,153,0.18)

Sidebar: Active items use green highlight and subtle glow. Hover tint
shifts toward green.

Background: Very subtle gradient: deep navy → dark teal.

Placeholder Text Examples: Workflows: "Nothing growing here yet. Create
your first workflow." Connections: "Link integrations to your system."
Agents: "Intelligence that can reason and act inside workflows." Event
Stream: "Watch signals move through your system." Runs: "Observe
workflows as they execute."

Completion Criteria: - Primary color switched to green - Glow changed
from violet to green - Workflow color tokens added - Sidebar highlight
updated - Placeholder copy adjusted - Tailwind + shadcn tokens updated
