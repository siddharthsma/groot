import { AgentsPanel } from "@/components/agents/AgentsPanel";
import { AppShell } from "@/components/layout/AppShell";

export default function AgentsPage() {
  return (
    <AppShell
      title="Agents"
      description="Agent and session management UI will build on this route in later phases."
    >
      <AgentsPanel />
    </AppShell>
  );
}
