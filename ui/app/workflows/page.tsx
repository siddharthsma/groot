import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function WorkflowsPage() {
  return (
    <AppShell
      title="Workflows"
      description="Nothing growing here yet. Create your first workflow."
    >
      <PlaceholderPanel
        title="Workflow space is ready to expand."
        description="This route will later host the visual builder, validation feedback, and publish controls. Right now it simply marks where new automation paths will grow."
        eyebrow="Build"
      />
    </AppShell>
  );
}
