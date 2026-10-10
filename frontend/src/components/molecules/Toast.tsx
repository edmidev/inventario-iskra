import * as RadixToast from "@radix-ui/react-toast";
import clsx from "clsx";
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

type ToastTone = "success" | "error";
interface ToastMessage {
  id: number;
  tone: ToastTone;
  text: string;
}

interface ToastApi {
  success: (text: string) => void;
  error: (text: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);
let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [messages, setMessages] = useState<ToastMessage[]>([]);
  const push = useCallback((tone: ToastTone, text: string) => {
    setMessages((current) => [...current, { id: nextId++, tone, text }]);
  }, []);
  const api = useMemo<ToastApi>(
    () => ({ success: (text) => push("success", text), error: (text) => push("error", text) }),
    [push],
  );

  return (
    <ToastContext.Provider value={api}>
      <RadixToast.Provider swipeDirection="down" duration={4000}>
        {children}
        {messages.map((message) => (
          <RadixToast.Root
            key={message.id}
            onOpenChange={(open) => {
              if (!open) setMessages((current) => current.filter((m) => m.id !== message.id));
            }}
            className={clsx(
              "rounded-xl border px-4 py-3 text-sm font-medium shadow-lg",
              message.tone === "success" ? "border-ok/40 bg-ok-soft text-ok" : "border-bad/40 bg-bad-soft text-bad",
            )}
          >
            <RadixToast.Description>{message.text}</RadixToast.Description>
          </RadixToast.Root>
        ))}
        <RadixToast.Viewport label="Notificaciones ({hotkey})" className="fixed inset-x-0 bottom-24 z-[70] mx-auto flex w-full max-w-md flex-col gap-2 px-4 outline-none md:bottom-6" />
      </RadixToast.Provider>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used inside ToastProvider");
  return context;
}
