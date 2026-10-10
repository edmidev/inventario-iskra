import type { ReactNode } from "react";
import { createPortal } from "react-dom";

/**
 * Content that exists only on paper. It is portaled to <body> so the print stylesheet can
 * hide the whole app (`#root`, dialogs, toasts) and show just this block.
 */
export function PrintArea({ children }: { children: ReactNode }) {
  return createPortal(<div className="print-root">{children}</div>, document.body);
}
