import { Building2, Sparkles } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { tenantPlaceholder } from "@/lib/theme/tokens";

export function TenantBadge() {
  return (
    <div className="shell-panel flex items-center justify-between gap-3 px-3 py-3">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary/15 text-primary shadow-[var(--shadow-glow-soft)]">
          <Building2 className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-foreground">
            {tenantPlaceholder.name}
          </p>
          <p className="text-xs text-muted-foreground">Tenant context</p>
        </div>
      </div>
      <Badge variant="secondary" className="gap-1 border-border/60 bg-surface-3/80 text-secondary-foreground">
        <Sparkles className="h-3 w-3" />
        {tenantPlaceholder.environment}
      </Badge>
    </div>
  );
}
