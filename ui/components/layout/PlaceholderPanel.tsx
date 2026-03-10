import { ArrowUpRight, Sparkles } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

type PlaceholderPanelProps = {
  title: string;
  description: string;
  eyebrow?: string;
};

export function PlaceholderPanel({
  title,
  description,
  eyebrow = "TBC",
}: PlaceholderPanelProps) {
  return (
    <Card className="shell-panel shell-panel-hover overflow-hidden rounded-[var(--radius-shell)] border-border/70 bg-surface-1/88 shadow-[var(--shadow-panel)]">
      <CardHeader className="border-b border-border/60 pb-5">
        <div className="flex flex-wrap items-center gap-3">
          <Badge className="gap-1 rounded-full border-border/60 bg-primary/12 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-primary">
            <Sparkles className="h-3 w-3" />
            {eyebrow}
          </Badge>
          <div className="h-px flex-1 bg-gradient-to-r from-primary/40 via-accent/18 to-transparent" />
        </div>
        <CardTitle className="pt-4 text-2xl font-semibold tracking-[-0.02em] text-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-6 py-6">
        <p className="max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
          {description}
        </p>
        <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
          <div className="rounded-[calc(var(--radius-shell)-0.25rem)] border border-border/70 bg-surface-2/78 p-5 shadow-[var(--shadow-inset-soft)]">
            <p className="text-sm font-medium text-foreground">Shell ready for real signals</p>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              This route now uses the shared Groot shell, token system, and
              placeholder treatment that future product pages will inherit.
            </p>
          </div>
          <div className="rounded-[calc(var(--radius-shell)-0.25rem)] border border-dashed border-primary/30 bg-gradient-to-br from-primary/10 via-accent/6 to-transparent p-5">
            <div className="flex items-center justify-between text-sm font-medium text-foreground">
              <span>Next phase</span>
              <ArrowUpRight className="h-4 w-4 text-primary" />
            </div>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              Real content, forms, lists, and workflow interactions stay out of
              this phase. This panel remains intentionally structural.
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
