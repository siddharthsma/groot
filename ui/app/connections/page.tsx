import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function ConnectionsPage() {
  return (
    <AppShell
      title="Connections"
      description="Link integrations to your system."
    >
      <PlaceholderPanel
        title="Connections will anchor live signals."
        description="This route will later hold connection lists, setup flows, and configuration detail. For now it marks the place where integrations attach to your system."
        eyebrow="Source"
      />
    </AppShell>
  );
}
