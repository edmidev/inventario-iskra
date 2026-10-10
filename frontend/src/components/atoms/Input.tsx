import clsx from "clsx";
import { forwardRef, type InputHTMLAttributes, type TextareaHTMLAttributes } from "react";

const base =
  "block w-full rounded-xl border bg-surface px-3.5 text-base text-ink placeholder:text-ink-soft/70 " +
  "disabled:cursor-not-allowed disabled:opacity-60";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(
  function Input({ invalid, className, ...props }, ref) {
    return (
      <input
        ref={ref}
        aria-invalid={invalid || undefined}
        className={clsx(base, "min-h-11", invalid ? "border-bad" : "border-line", className)}
        {...props}
      />
    );
  },
);

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }>(
  function Textarea({ invalid, className, ...props }, ref) {
    return (
      <textarea
        ref={ref}
        aria-invalid={invalid || undefined}
        className={clsx(base, "min-h-24 py-2.5", invalid ? "border-bad" : "border-line", className)}
        {...props}
      />
    );
  },
);
