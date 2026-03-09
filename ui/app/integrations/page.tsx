import { IntegrationsPanel } from "@/components/integrations/IntegrationsPanel";
import { AppShell } from "@/components/layout/AppShell";

export default function IntegrationsPage() {
  return (
    <AppShell
      title="Integrations"
      description="Integration catalog and configuration screens will land here in later phases."
    >
      <IntegrationsPanel />
    </AppShell>
  );
}
