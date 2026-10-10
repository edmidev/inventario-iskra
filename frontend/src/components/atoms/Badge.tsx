import clsx from "clsx";
import type { ReactNode } from "react";

export type BadgeTone = "neutral" | "accent" | "ok" | "bad" | "warn" | "low";

const tones: Record<BadgeTone, string> = {
  neutral: "bg-sunken text-ink-soft",
  accent: "bg-accent-soft text-accent",
  ok: "bg-ok-soft text-ok",
  bad: "bg-bad-soft text-bad",
  warn: "bg-warn-soft text-warn",
  low: "bg-low-soft text-low",
};

export function Badge({ tone = "neutral", children }: { tone?: BadgeTone; children: ReactNode }) {
  return (
    <span
      className={clsx(
        "inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold whitespace-nowrap",
        tones[tone],
      )}
    >
      {children}
    </span>
  );
}
