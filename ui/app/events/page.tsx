import { EventsPanel } from "@/components/events/EventsPanel";
import { AppShell } from "@/components/layout/AppShell";

export default function EventsPage() {
  return (
    <AppShell
      title="Events"
      description="Event browsing, execution graphs, and replay tooling will build on this route."
    >
      <EventsPanel />
    </AppShell>
  );
}
