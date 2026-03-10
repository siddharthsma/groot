import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function RunsPage() {
  return (
    <AppShell
      title="Runs"
      description="Observe workflows as they execute."
    >
      <PlaceholderPanel
        title="Run history will appear here."
        description="This route will later show execution timelines, wait states, and runtime traces. For now it keeps the monitoring surface aligned with the rest of the shell."
        eyebrow="Monitor"
      />
    </AppShell>
  );
}
