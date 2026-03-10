"use client";

import { Orbit, Sparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { SidebarNav } from "@/components/layout/SidebarNav";
import { TenantBadge } from "@/components/layout/TenantBadge";

type SidebarProps = {
  compact?: boolean;
};

export function Sidebar({ compact = false }: SidebarProps) {
  return (
    <aside className="flex h-full flex-col gap-6 border-r border-border/60 bg-sidebar/88 px-4 py-5 backdrop-blur-2xl lg:px-5">
      <div className="space-y-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-primary/20 bg-gradient-to-br from-primary/18 via-primary/6 to-accent/10 text-primary shadow-[var(--shadow-glow-soft)]">
              <Orbit className="h-5 w-5" />
            </div>
            <div>
              <p className="text-[10px] font-bold uppercase tracking-[0.34em] text-muted-foreground/80">
                Groot
              </p>
              <h1 className="text-lg font-bold tracking-[-0.05em] text-foreground">
                Cosmic Shell
              </h1>
            </div>
          </div>
          <Badge className="gap-1 border-primary/25 bg-primary/12 text-primary">
            <Sparkles className="h-3 w-3" />
            Phase 38B
          </Badge>
        </div>
        {!compact && <TenantBadge />}
      </div>
      <SidebarNav />
      <div className="mt-auto rounded-[calc(var(--radius-panel)-0.125rem)] border border-border/60 bg-surface-2/70 p-4">
        <p className="text-sm font-semibold tracking-[-0.02em] text-foreground">
          The system is ready to grow
        </p>
        <p className="mt-2 text-[13px] leading-6 text-muted-foreground">
          Navigation, theme tokens, and responsive layout are stable. Product
          content will grow into these surfaces in later phases.
        </p>
      </div>
    </aside>
  );
}
