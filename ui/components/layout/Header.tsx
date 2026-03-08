import { Badge } from "@/components/ui/badge";

type HeaderProps = {
  title: string;
  description: string;
};

export function Header({ title, description }: HeaderProps) {
  return (
    <header className="border-b border-slate-200/80 bg-white/70 px-5 py-5 backdrop-blur sm:px-8">
      <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div className="space-y-1">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-500">
            Frontend Scaffold
          </p>
          <h2 className="text-3xl font-semibold tracking-tight text-slate-950">
            {title}
          </h2>
          <p className="max-w-3xl text-sm leading-6 text-slate-600">{description}</p>
        </div>
        <Badge className="w-fit border border-emerald-200 bg-emerald-50 text-emerald-700 hover:bg-emerald-50">
          Groot UI initialized
        </Badge>
      </div>
    </header>
  );
}
