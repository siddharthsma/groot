"use client";

import type { ShellNavSection as NavSection } from "@/lib/theme/tokens";
import { SidebarNavItem } from "@/components/layout/SidebarNavItem";

type SidebarNavSectionProps = {
  section: NavSection;
};

export function SidebarNavSection({ section }: SidebarNavSectionProps) {
  return (
    <section className="space-y-2">
      <p className="px-3 text-[11px] font-semibold uppercase tracking-[0.28em] text-muted-foreground/80">
        {section.label}
      </p>
      <div className="space-y-1">
        {section.items.map((item) => (
          <SidebarNavItem key={item.href} item={item} />
        ))}
      </div>
    </section>
  );
}
