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
    <Card className="border-slate-200/80 bg-white/90 shadow-sm">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="text-sm leading-6 text-slate-600">
        {description}
      </CardContent>
    </Card>
  );
}
