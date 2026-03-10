"use client";

import { Command, Menu, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { ShellContainer } from "@/components/layout/ShellContainer";
import { Sidebar } from "@/components/layout/Sidebar";

type TopBarProps = {
  title: string;
};

export function TopBar({ title }: TopBarProps) {
  return (
    <header className="sticky top-0 z-30 border-b border-border/60 bg-background/70 backdrop-blur-xl">
      <ShellContainer className="flex h-16 items-center gap-3">
        <div className="lg:hidden">
          <Sheet>
            <SheetTrigger
              className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border/60 bg-surface-2/80 text-foreground transition-colors hover:bg-surface-3/80 focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40"
              aria-label="Open navigation"
            >
              <Menu className="h-4 w-4" />
            </SheetTrigger>
            <SheetContent
              side="left"
              className="border-border/60 bg-background/96 p-0 shadow-[var(--shadow-panel-strong)]"
            >
              <SheetHeader className="border-b border-border/60 px-4 py-4">
                <SheetTitle className="text-left text-foreground">Groot</SheetTitle>
              </SheetHeader>
              <Sidebar compact />
            </SheetContent>
          </Sheet>
        </div>

        <div className="min-w-0 flex-1">
          <p className="text-[10px] font-bold uppercase tracking-[0.3em] text-muted-foreground/70">
            Tenant workspace
          </p>
          <h1 className="truncate text-sm font-bold tracking-[-0.03em] text-foreground sm:text-base">
            {title}
          </h1>
        </div>

        <div className="hidden min-w-[280px] flex-1 items-center justify-center lg:flex">
          <div className="relative w-full max-w-md">
            <Input
              readOnly
              aria-label="Command search placeholder"
              value=""
              placeholder="Search or jump to anything"
              className="h-11 rounded-2xl border-border/60 bg-surface-2/85 pl-11 pr-24 text-sm text-foreground shadow-[var(--shadow-inset-soft)] placeholder:text-muted-foreground/70"
            />
            <Command className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground/70" />
            <div className="pointer-events-none absolute right-3 top-1/2 flex -translate-y-1/2 items-center gap-1 rounded-xl border border-border/60 bg-surface-3/90 px-2 py-1 text-[11px] font-medium text-muted-foreground">
              <span>Command</span>
              <span className="rounded-md bg-background/80 px-1.5 py-0.5 text-foreground/80">K</span>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="outline" className="hidden border-border/60 bg-surface-2/85 lg:inline-flex">
            Action Placeholder
          </Button>
          <Button className="h-11 rounded-2xl px-4 shadow-[var(--shadow-glow-soft)]">
            <Plus className="h-4 w-4" />
            New
          </Button>
        </div>
      </ShellContainer>
    </header>
  );
}
