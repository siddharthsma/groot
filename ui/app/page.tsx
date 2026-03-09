import { GraphCanvas } from "@/components/graphs/GraphCanvas";
import { AppShell } from "@/components/layout/AppShell";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function Home() {
  return (
    <AppShell
      title="Workspace Overview"
      description="Groot UI initialized. This Phase 31 shell provides the app router, shared layout, query client, form scaffolding, and graph canvas foundation for later product screens."
    >
      <div className="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
        <Card className="overflow-hidden border-slate-200/80 bg-white/90 shadow-sm">
          <CardHeader>
            <CardTitle>Graph Foundation</CardTitle>
          </CardHeader>
          <CardContent>
            <GraphCanvas />
          </CardContent>
        </Card>
        <Card className="border-slate-200/80 bg-white/90 shadow-sm">
          <CardHeader>
            <CardTitle>What Exists Now</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm text-slate-600">
            <p>Next.js App Router workspace with TypeScript and Tailwind.</p>
            <p>React Query integration and API client foundation.</p>
            <p>Placeholder routes for integrations, events, and agents.</p>
            <p>shadcn/ui base components plus reusable form scaffolding.</p>
          </CardContent>
        </Card>
      </div>
    </AppShell>
  );
}
