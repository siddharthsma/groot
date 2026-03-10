import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function EventsPage() {
  return (
    <AppShell
      title="Event Stream"
      description="Watch signals move through your system."
    >
      <PlaceholderPanel
        title="The stream is quiet for now."
        description="This route will later surface events, replay controls, and graph-level inspection. Today it simply frames where system signals will be observed."
        eyebrow="Monitor"
      />
    </AppShell>
  );
}
