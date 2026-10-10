import * as Dialog from "@radix-ui/react-dialog";
import type { ReactNode } from "react";
import { IconButton } from "../atoms/IconButton";

interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
}

/** Bottom sheet on phones, centered dialog from md up. Radix handles focus trap, Esc and aria. */
export function Sheet({ open, onOpenChange, title, description, children }: SheetProps) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="sheet-overlay fixed inset-0 z-40 bg-overlay" />
        <Dialog.Content
          // Keep focus on the sheet itself: auto-focusing the close button pops its tooltip open.
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            (event.currentTarget as HTMLElement).focus();
          }}
          className={
            "sheet-content fixed inset-x-0 bottom-0 z-50 flex max-h-[92dvh] flex-col rounded-t-3xl bg-surface shadow-sheet " +
            "md:inset-x-auto md:bottom-auto md:left-1/2 md:top-1/2 md:w-full md:max-w-lg md:-translate-x-1/2 md:-translate-y-1/2 md:rounded-3xl"
          }
        >
          <div className="mx-auto mt-2 h-1.5 w-10 rounded-full bg-line md:hidden" aria-hidden="true" />
          <header className="flex items-start justify-between gap-2 px-5 pt-3 pb-2">
            <div>
              <Dialog.Title className="text-lg font-semibold">{title}</Dialog.Title>
              {description ? (
                <Dialog.Description className="text-sm text-ink-soft">{description}</Dialog.Description>
              ) : (
                <Dialog.Description className="sr-only">{title}</Dialog.Description>
              )}
            </div>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Cerrar" className="-mr-2 shrink-0" />
            </Dialog.Close>
          </header>
          <div className="overflow-y-auto px-5 pt-1 pb-[max(1.25rem,env(safe-area-inset-bottom))]">{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
