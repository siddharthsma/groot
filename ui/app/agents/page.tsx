import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function AgentsPage() {
  return (
    <AppShell
      title="Agents"
      description="Intelligence that can reason and act inside workflows."
    >
      <PlaceholderPanel
        title="Agents will give workflows decision-making power."
        description="This route will later hold agent definitions, versions, and live session visibility. For now it keeps the shell ready for intelligence to take shape."
        eyebrow="Build"
      />
    </AppShell>
  );
}
