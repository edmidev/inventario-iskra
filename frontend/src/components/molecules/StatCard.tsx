import clsx from "clsx";
import { Link } from "react-router-dom";

interface StatCardProps {
  label: string;
  value: number;
  tone?: "default" | "ok" | "bad" | "warn" | "low";
  to?: string;
}

const tones = { default: "text-ink", ok: "text-ok", bad: "text-bad", warn: "text-warn", low: "text-low" };

export function StatCard({ label, value, tone = "default", to }: StatCardProps) {
  const body = (
    <>
      <span className={clsx("text-3xl font-bold tabular-nums", tones[tone])}>{value}</span>
      <span className="text-sm text-ink-soft">{label}</span>
    </>
  );
  const classes = "flex min-h-24 flex-col justify-between rounded-card border border-line bg-surface p-4";
  return to ? (
    <Link to={to} className={clsx(classes, "transition-colors hover:bg-sunken")}>
      {body}
    </Link>
  ) : (
    <div className={classes}>{body}</div>
  );
}
