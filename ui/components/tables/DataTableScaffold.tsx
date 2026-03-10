import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

type DataTableScaffoldProps = {
  title: string;
  description: string;
};

export function DataTableScaffold({
  title,
  description,
}: DataTableScaffoldProps) {
  return (
    <Card className="shell-panel rounded-[var(--radius-panel)] border-border/70 bg-surface-1/88 shadow-[var(--shadow-panel)]">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="text-sm leading-7 text-muted-foreground">
        {description}
      </CardContent>
    </Card>
  );
}
