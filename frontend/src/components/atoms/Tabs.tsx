import * as RadixTabs from "@radix-ui/react-tabs";
import clsx from "clsx";
import type { ReactNode } from "react";

export const Tabs = RadixTabs.Root;

export function TabsList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <RadixTabs.List aria-label={label} className="flex border-b border-line">
      {children}
    </RadixTabs.List>
  );
}

export function TabsTrigger({ value, children }: { value: string; children: ReactNode }) {
  return (
    <RadixTabs.Trigger
      value={value}
      className={clsx(
        "-mb-px min-h-11 flex-1 border-b-2 border-transparent px-3 text-sm font-semibold text-ink-soft",
        "data-[state=active]:border-accent data-[state=active]:text-accent",
      )}
    >
      {children}
    </RadixTabs.Trigger>
  );
}

export const TabsContent = RadixTabs.Content;
