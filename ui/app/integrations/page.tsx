import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function IntegrationsPage() {
  return (
    <AppShell
      title="Integrations"
      description="Explore integrations your system can connect to. Add connections to bring signals into your workflows."
    >
      <PlaceholderPanel
        title="Integration discovery starts here."
        description="This space will later hold integration metadata, install flows, and package details. For now it shows where new signals and destinations will enter the system."
        eyebrow="Source"
      />
    </AppShell>
  );
}
