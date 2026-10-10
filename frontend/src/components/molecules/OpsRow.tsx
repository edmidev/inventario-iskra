import type { ReactNode } from "react";
import { Link } from "react-router-dom";

interface OpsRowProps {
  itemId: string;
  code: string;
  description: string;
  /** Extra lines under the description. */
  children?: ReactNode;
  /** Status badges / figures shown top-right. */
  aside?: ReactNode;
  /** Quick action (kept outside the link so it is its own tap target). */
  action?: ReactNode;
}

/** A tappable row for operational lists: the item link plus an optional one-tap action. */
export function OpsRow({ itemId, code, description, children, aside, action }: OpsRowProps) {
  return (
    <li className="flex flex-col gap-2 rounded-card border border-line bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <Link to={`/materiales/${itemId}`} className="min-h-11 min-w-0 flex-1">
          <span className="block font-mono text-sm font-semibold text-accent">{code}</span>
          <span className="block font-medium">{description}</span>
        </Link>
        {aside && <div className="flex shrink-0 flex-col items-end gap-1 text-right">{aside}</div>}
      </div>
      {children && <div className="flex flex-col gap-0.5 text-sm text-ink-soft">{children}</div>}
      {action}
    </li>
  );
}
