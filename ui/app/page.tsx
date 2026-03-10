import { AppShell } from "@/components/layout/AppShell";
import { PlaceholderPanel } from "@/components/layout/PlaceholderPanel";

export default function Home() {
  return (
    <AppShell
      title="Overview"
      description="A living system of events, connections, and workflows. Watch your automations grow."
    >
      <PlaceholderPanel
        title="Overview is ready to gather real system signals."
        description="This surface will eventually hold workflow posture, run health, and event activity. For now it frames the shell around a system that is ready to come to life."
        eyebrow="Overview"
      />
    </AppShell>
  );
}
