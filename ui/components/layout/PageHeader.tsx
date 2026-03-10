type PageHeaderProps = {
  title: string;
  description: string;
};

export function PageHeader({ title, description }: PageHeaderProps) {
  return (
    <div className="space-y-3.5">
      <p className="text-[11px] font-bold uppercase tracking-[0.3em] text-primary/80">
        Groot UI
      </p>
      <div className="space-y-2.5">
        <h2 className="text-3xl font-bold tracking-[-0.05em] text-foreground sm:text-4xl">
          {title}
        </h2>
        <p className="max-w-3xl text-[15px] leading-8 text-muted-foreground sm:text-base">
          {description}
        </p>
      </div>
    </div>
  );
}
