import type { User } from "./schemas";

/**
 * Access token lives in module memory only: never localStorage/sessionStorage.
 * A page reload recovers the session through the httpOnly refresh cookie.
 */
let accessToken: string | null = null;

export type SessionEvent = { type: "session"; user: User } | { type: "expired" };
type Listener = (event: SessionEvent) => void;
const listeners = new Set<Listener>();

export const tokenStore = {
  get: () => accessToken,
  set: (token: string | null) => {
    accessToken = token;
  },
};

export function subscribeSession(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function emitSession(event: SessionEvent): void {
  listeners.forEach((listener) => listener(event));
}
