import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type ShellContainerProps = {
  children: ReactNode;
  className?: string;
};

export function ShellContainer({ children, className }: ShellContainerProps) {
  return (
    <div className={cn("mx-auto w-full max-w-[1440px] px-4 sm:px-6 lg:px-8", className)}>
      {children}
    </div>
  );
}
