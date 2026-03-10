"use client";

import { SidebarNavSection } from "@/components/layout/SidebarNavSection";
import { shellNavSections } from "@/lib/theme/tokens";

export function SidebarNav() {
  return (
    <nav aria-label="Primary" className="space-y-5">
      {shellNavSections.map((section) => (
        <SidebarNavSection key={section.label} section={section} />
      ))}
    </nav>
  );
}
