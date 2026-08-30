import { ReactNode } from "react";

export function PageHeader({
  title,
  subtitle,
  action,
}: {
  title: string;
  subtitle?: string;
  action?: ReactNode;
}) {
  return (
    <div className="tuma-page-header">
      <div>
        <h1 className="tuma-page-title">{title}</h1>
        {subtitle && <p className="tuma-page-sub">{subtitle}</p>}
      </div>
      {action}
    </div>
  );
}
