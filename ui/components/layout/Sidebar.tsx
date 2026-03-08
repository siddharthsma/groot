import Link from "next/link";
import { Bot, Boxes, Cable, Workflow } from "lucide-react";
import { Badge } from "@/components/ui/badge";

const items = [
  { href: "/", label: "Overview", icon: Workflow },
  { href: "/providers", label: "Providers", icon: Boxes },
  { href: "/events", label: "Events", icon: Cable },
  { href: "/agents", label: "Agents", icon: Bot },
];

export function Sidebar() {
  return (
    <aside className="border-r border-slate-200/80 bg-white/80 px-4 py-5 backdrop-blur lg:px-5">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.22em] text-slate-500">
            Groot
          </p>
          <h1 className="mt-1 text-xl font-semibold tracking-tight text-slate-950">
            UI Workspace
          </h1>
        </div>
        <Badge variant="secondary" className="border border-slate-200 bg-slate-50">
          Phase 31
        </Badge>
      </div>
      <nav className="mt-8 space-y-2">
        {items.map(({ href, label, icon: Icon }) => (
          <Link
            key={href}
            href={href}
            className="flex items-center gap-3 rounded-xl border border-transparent px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-200 hover:bg-slate-50 hover:text-slate-950"
          >
            <Icon className="h-4 w-4" />
            {label}
          </Link>
        ))}
      </nav>
    </aside>
  );
}
