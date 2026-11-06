import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { login, logout, refreshSession } from "../../lib/api/client";
import { ApiError } from "../../lib/api/errors";
import type { Role, User } from "../../lib/api/schemas";
import { subscribeSession } from "../../lib/api/session-store";

type SessionState =
  | { status: "loading"; user: null }
  | { status: "anonymous"; user: null }
  /** The refresh call could not reach the server: the session may still be valid, so do not log out. */
  | { status: "unreachable"; user: null }
  | { status: "authenticated"; user: User };

interface SessionContextValue {
  state: SessionState;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  /** Retries recovering the session after a network failure. */
  retry: () => void;
}

const SessionContext = createContext<SessionContextValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<SessionState>({ status: "loading", user: null });

  const recover = useCallback(() => {
    // Recover the session from the httpOnly refresh cookie; concurrent calls are deduplicated.
    refreshSession()
      .then((session) => {
        if (!session) setState({ status: "anonymous", user: null });
      })
      .catch((error: unknown) => {
        // Offline (or the API is down): keep the cookie and let the user retry instead of logging out.
        setState(error instanceof ApiError && error.code === "network" ? { status: "unreachable", user: null } : { status: "anonymous", user: null });
      });
  }, []);

  const retry = useCallback(() => {
    setState({ status: "loading", user: null });
    recover();
  }, [recover]);

  useEffect(() => {
    const unsubscribe = subscribeSession((event) => {
      if (event.type === "session") {
        setState({ status: "authenticated", user: event.user });
      } else {
        setState({ status: "anonymous", user: null });
        queryClient.clear();
      }
    });
    recover();
    return unsubscribe;
  }, [queryClient, recover]);

  const signIn = useCallback(async (email: string, password: string) => {
    await login(email, password);
  }, []);

  const signOut = useCallback(async () => {
    await logout();
  }, []);

  const value = useMemo(() => ({ state, signIn, signOut, retry }), [state, signIn, signOut, retry]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const context = useContext(SessionContext);
  if (!context) throw new Error("useSession must be used inside SessionProvider");
  return context;
}

export interface Permissions {
  role: Role | null;
  canRead: boolean;
  canWriteItems: boolean;
  canOperate: boolean;
  canManageUsers: boolean;
  canManageRoles: boolean;
  canManageCatalogs: boolean;
  /** Compatibility flag for existing custom-code controls, governed by catalogs:manage. */
  isAdmin: boolean;
}

export function permissionsFor(user: Pick<User, "role" | "permissions"> | null): Permissions {
  const has = (permission: string) => user?.permissions.includes(permission) ?? false;
  return {
    role: user?.role ?? null,
    canRead: has("inventory:read"),
    canWriteItems: has("items:write"),
    canOperate: has("operations:write"),
    canManageUsers: has("users:manage"),
    canManageRoles: has("roles:manage"),
    canManageCatalogs: has("catalogs:manage"),
    isAdmin: has("catalogs:manage"),
  };
}

export function usePermissions(): Permissions {
  const { state } = useSession();
  return permissionsFor(state.user);
}
