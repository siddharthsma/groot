import type { ReactNode } from "react";
import { PageHeader } from "@/components/layout/PageHeader";
import { Sidebar } from "@/components/layout/Sidebar";
import { ShellContainer } from "@/components/layout/ShellContainer";
import { TopBar } from "@/components/layout/TopBar";

type AppShellProps = {
  title: string;
  description: string;
  children: ReactNode;
};

export function AppShell({ title, description, children }: AppShellProps) {
  return (
    <div className="min-h-screen bg-transparent text-foreground">
      <div className="mx-auto grid min-h-screen max-w-[1680px] grid-cols-1 lg:grid-cols-[296px_minmax(0,1fr)]">
        <div className="hidden lg:block">
          <Sidebar />
        </div>
        <div className="relative flex min-w-0 flex-col">
          <TopBar title={title} />
          <main className="flex-1 pb-10 pt-8">
            <ShellContainer className="space-y-8">
              <PageHeader title={title} description={description} />
              {children}
            </ShellContainer>
          </main>
        </div>
      </div>
    </div>
  );
}
