"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ShellNavItem as NavItem } from "@/lib/theme/tokens";
import { cn } from "@/lib/utils";

type SidebarNavItemProps = {
  item: NavItem;
};

export function SidebarNavItem({ item }: SidebarNavItemProps) {
  const pathname = usePathname();
  const isActive = pathname === item.href;
  const Icon = item.icon;

  return (
    <Link
      href={item.href}
      aria-current={isActive ? "page" : undefined}
      className={cn("nav-item", isActive && "nav-item-active")}
    >
      <span className="nav-icon">
        <Icon className="h-4 w-4" />
      </span>
      <span className="truncate">{item.label}</span>
    </Link>
  );
}
