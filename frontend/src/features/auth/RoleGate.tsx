import type { ReactNode } from "react";
import { usePermissions, type Permissions } from "./session";

/** Renders children only when the predicate holds for the current role; otherwise hides them. */
export function Can({ check, children }: { check: (permissions: Permissions) => boolean; children: ReactNode }) {
  return check(usePermissions()) ? <>{children}</> : null;
}
