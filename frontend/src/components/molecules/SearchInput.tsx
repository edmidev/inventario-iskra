import { useEffect, useRef, useState } from "react";
import { Icon } from "../atoms/Icon";
import { Input } from "../atoms/Input";

interface SearchInputProps {
  value: string;
  onChange: (value: string) => void;
  label: string;
  placeholder?: string;
  debounceMs?: number;
}

/** Controlled-by-URL search box: keeps local text for typing and reports changes after a pause. */
export function SearchInput({ value, onChange, label, placeholder, debounceMs = 300 }: SearchInputProps) {
  const [text, setText] = useState(value);
  const lastReported = useRef(value);

  // Follow external changes (back/forward, "clear filters").
  useEffect(() => {
    if (value !== lastReported.current) {
      lastReported.current = value;
      setText(value);
    }
  }, [value]);

  useEffect(() => {
    if (text === lastReported.current) return;
    const timer = setTimeout(() => {
      lastReported.current = text;
      onChange(text);
    }, debounceMs);
    return () => clearTimeout(timer);
  }, [text, debounceMs, onChange]);

  return (
    <div className="relative">
      <Icon name="search" className="pointer-events-none absolute top-1/2 left-3.5 size-5 -translate-y-1/2 text-ink-soft" />
      <Input
        type="search"
        inputMode="search"
        aria-label={label}
        placeholder={placeholder}
        value={text}
        onChange={(event) => setText(event.target.value)}
        className="pl-11"
      />
    </div>
  );
}
