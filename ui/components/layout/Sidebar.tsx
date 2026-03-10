"use client";

import Image from "next/image";
import { SidebarNav } from "@/components/layout/SidebarNav";
import { TenantBadge } from "@/components/layout/TenantBadge";

type SidebarProps = {
  compact?: boolean;
};

export function Sidebar({ compact = false }: SidebarProps) {
  return (
    <aside className="flex h-full flex-col gap-6 border-r border-border/60 bg-sidebar/88 px-4 py-5 backdrop-blur-2xl lg:px-5">
      <div className="space-y-4">
        <div className="flex items-center gap-3 px-3">
          <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl border border-border/60 bg-surface-2/82 shadow-[0_14px_28px_rgba(3,8,28,0.22)]">
            <Image
              src="/groot_logo_trans.png"
              alt="Groot logo"
              width={133}
              height={133}
              className="h-[8.3rem] w-[8.3rem] object-contain"
              priority
            />
          </div>
          <h1 className="text-[1.35rem] font-semibold tracking-[-0.045em] text-foreground sm:text-[1.45rem]">
            Groot
          </h1>
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
