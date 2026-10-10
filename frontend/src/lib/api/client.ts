import type { z } from "zod";
import { ApiError } from "./errors";
import { apiErrorBodySchema, sessionSchema, type Session } from "./schemas";
import { emitSession, tokenStore } from "./session-store";

export const API_BASE = "/api/v1";

type QueryValue = string | number | boolean | null | undefined;

export interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Record<string, QueryValue>;
  /** Attach the bearer token and refresh once on 401. Defaults to true. */
  auth?: boolean;
  signal?: AbortSignal;
}

function buildUrl(path: string, query?: Record<string, QueryValue>): string {
  const url = new URL(`${API_BASE}${path}`, window.location.origin);
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== "") url.searchParams.set(key, String(value));
  }
  return url.toString();
}

async function toApiError(response: Response): Promise<ApiError> {
  try {
    const parsed = apiErrorBodySchema.safeParse(await response.json());
    if (parsed.success) {
      const { code, message, fields } = parsed.data.error;
      return new ApiError(response.status, code, message, fields);
    }
  } catch {
    // fall through to the generic error
  }
  return new ApiError(response.status, "internal_error", response.statusText || "Request failed");
}

async function send(path: string, options: RequestOptions, token: string | null): Promise<Response> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (token && options.auth !== false) headers.Authorization = `Bearer ${token}`;
  try {
    return await fetch(buildUrl(path, options.query), {
      method: options.method ?? "GET",
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      credentials: "include",
      signal: options.signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new ApiError(0, "network", "Network error");
  }
}

async function parse<T>(response: Response, schema: z.ZodType<T> | null): Promise<T> {
  if (schema === null || response.status === 204) return undefined as T;
  const parsed = schema.safeParse(await response.json().catch(() => undefined));
  if (!parsed.success) throw new ApiError(response.status, "invalid_response", "Unexpected response shape");
  return parsed.data;
}

let inflightRefresh: Promise<Session | null> | null = null;

/**
 * Rotates the refresh cookie once, even when several callers race. Resolves null
 * when the session is gone (401); other failures propagate.
 */
export function refreshSession(): Promise<Session | null> {
  inflightRefresh ??= (async () => {
    try {
      const response = await send("/auth/refresh", { method: "POST", auth: false }, null);
      if (response.status === 401) {
        tokenStore.set(null);
        return null;
      }
      if (!response.ok) throw await toApiError(response);
      const session = await parse(response, sessionSchema);
      tokenStore.set(session.access_token);
      emitSession({ type: "session", user: session.user });
      return session;
    } finally {
      inflightRefresh = null;
    }
  })();
  return inflightRefresh;
}

export async function request<T>(path: string, schema: z.ZodType<T> | null, options: RequestOptions = {}): Promise<T> {
  let response = await send(path, options, tokenStore.get());

  if (response.status === 401 && options.auth !== false) {
    const session = await refreshSession();
    if (!session) {
      emitSession({ type: "expired" });
      throw await toApiError(response);
    }
    response = await send(path, options, session.access_token);
    if (response.status === 401) {
      tokenStore.set(null);
      emitSession({ type: "expired" });
    }
  }

  if (!response.ok) throw await toApiError(response);
  return parse(response, schema);
}

export async function login(email: string, password: string): Promise<Session> {
  const session = await request("/auth/login", sessionSchema, {
    method: "POST",
    body: { email, password },
    auth: false,
  });
  tokenStore.set(session.access_token);
  emitSession({ type: "session", user: session.user });
  return session;
}

export async function logout(): Promise<void> {
  try {
    await request("/auth/logout", null, { method: "POST", auth: false });
  } finally {
    tokenStore.set(null);
    emitSession({ type: "expired" });
  }
}
