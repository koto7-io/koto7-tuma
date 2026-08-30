import { ReactNode } from "react";

export function Label({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={`tuma-label${className ? ` ${className}` : ""}`}>{children}</span>;
}
