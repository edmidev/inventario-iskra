import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { login, logout, refreshSession } from "../../lib/api/client";
import type { Role, User } from "../../lib/api/schemas";
import { subscribeSession } from "../../lib/api/session-store";

type SessionState =
  | { status: "loading"; user: null }
  | { status: "anonymous"; user: null }
  | { status: "authenticated"; user: User };

interface SessionContextValue {
  state: SessionState;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<SessionState>({ status: "loading", user: null });

  useEffect(() => {
    const unsubscribe = subscribeSession((event) => {
      if (event.type === "session") {
        setState({ status: "authenticated", user: event.user });
      } else {
        setState({ status: "anonymous", user: null });
        queryClient.clear();
      }
    });
    // Recover the session from the httpOnly refresh cookie; concurrent calls are deduplicated.
    refreshSession()
      .then((session) => {
        if (!session) setState({ status: "anonymous", user: null });
      })
      .catch(() => setState({ status: "anonymous", user: null }));
    return unsubscribe;
  }, [queryClient]);

  const signIn = useCallback(async (email: string, password: string) => {
    await login(email, password);
  }, []);

  const signOut = useCallback(async () => {
    await logout();
  }, []);

  const value = useMemo(() => ({ state, signIn, signOut }), [state, signIn, signOut]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const context = useContext(SessionContext);
  if (!context) throw new Error("useSession must be used inside SessionProvider");
  return context;
}

export interface Permissions {
  role: Role | null;
  /** operator and admin: create, edit and delete items. */
  canWriteItems: boolean;
  /** operator and admin: stock movements, loans and maintenance flows. */
  canOperate: boolean;
  /** admin only: catalogs, users and custom item codes. */
  isAdmin: boolean;
}

export function permissionsFor(role: Role | null): Permissions {
  return {
    role,
    canWriteItems: role === "operator" || role === "admin",
    canOperate: role === "operator" || role === "admin",
    isAdmin: role === "admin",
  };
}

export function usePermissions(): Permissions {
  const { state } = useSession();
  return permissionsFor(state.user?.role ?? null);
}
