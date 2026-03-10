import {
  Activity,
  Bot,
  Cable,
  Boxes,
  LayoutDashboard,
  type LucideIcon,
  Orbit,
  Workflow,
} from "lucide-react";

export type ShellNavItem = {
  href: string;
  label: string;
  icon: LucideIcon;
};

export type ShellNavSection = {
  label: string;
  items: ShellNavItem[];
};

export const shellNavSections: ShellNavSection[] = [
  {
    label: "Overview",
    items: [{ href: "/", label: "Overview", icon: LayoutDashboard }],
  },
  {
    label: "Source",
    items: [
      { href: "/integrations", label: "Integrations", icon: Boxes },
      { href: "/connections", label: "Connections", icon: Cable },
    ],
  },
  {
    label: "Build",
    items: [
      { href: "/workflows", label: "Workflows", icon: Workflow },
      { href: "/agents", label: "Agents", icon: Bot },
    ],
  },
  {
    label: "Monitor",
    items: [
      { href: "/events", label: "Event Stream", icon: Orbit },
      { href: "/runs", label: "Runs", icon: Activity },
    ],
  },
];

export const waitStrategies = [
  "event_id",
  "payload.<path>",
  "source.connection_id",
] as const;

export const tenantPlaceholder = {
  name: "Acme Tenant",
  environment: "Internal",
} as const;
