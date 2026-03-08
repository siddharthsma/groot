import { ProvidersPanel } from "@/components/providers/ProvidersPanel";
import { AppShell } from "@/components/layout/AppShell";

export default function ProvidersPage() {
  return (
    <AppShell
      title="Providers"
      description="Provider catalog and configuration screens will land here in later phases."
    >
      <ProvidersPanel />
    </AppShell>
  );
}
