import { ConnectionsPanel } from "@/components/connections/ConnectionsPanel";
import { AppShell } from "@/components/layout/AppShell";

export default function ConnectionsPage() {
  return (
    <AppShell
      title="Connections"
      description="Connection management screens will land here in later phases."
    >
      <ConnectionsPanel />
    </AppShell>
  );
}
