import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { api, errorBody } from "../../test/api";
import { session } from "../../test/fixtures";
import { z } from "zod";
import { login, request } from "./client";
import { ApiError, describeError } from "./errors";
import { emitSession, subscribeSession, tokenStore, type SessionEvent } from "./session-store";

const okSchema = z.object({ ok: z.boolean() });

describe("api client", () => {
  it("keeps the access token in memory only", async () => {
    server.use(http.post(api("/auth/login"), () => HttpResponse.json(session("admin"))));
    await login("a@b.co", "secret-password");
    expect(tokenStore.get()).toBe("token-admin");
    expect(JSON.stringify({ ...localStorage })).not.toContain("token-admin");
    expect(JSON.stringify({ ...sessionStorage })).not.toContain("token-admin");
  });

  it("sends the bearer token and credentials-bearing requests", async () => {
    let auth: string | null = null;
    server.use(
      http.get(api("/ping"), ({ request: req }) => {
        auth = req.headers.get("authorization");
        return HttpResponse.json({ ok: true });
      }),
    );
    tokenStore.set("abc");
    await expect(request("/ping", okSchema)).resolves.toEqual({ ok: true });
    expect(auth).toBe("Bearer abc");
  });

  it("refreshes silently once on 401 and retries the original request", async () => {
    const calls: (string | null)[] = [];
    server.use(
      http.get(api("/ping"), ({ request: req }) => {
        const auth = req.headers.get("authorization");
        calls.push(auth);
        return auth === "Bearer token-operator"
          ? HttpResponse.json({ ok: true })
          : HttpResponse.json(errorBody("unauthorized", "expired"), { status: 401 });
      }),
      http.post(api("/auth/refresh"), () => HttpResponse.json(session("operator"))),
    );
    tokenStore.set("stale");
    await expect(request("/ping", okSchema)).resolves.toEqual({ ok: true });
    expect(calls).toEqual(["Bearer stale", "Bearer token-operator"]);
    expect(tokenStore.get()).toBe("token-operator");
  });

  it("shares a single refresh between concurrent 401s", async () => {
    let refreshes = 0;
    server.use(
      http.get(api("/ping"), ({ request: req }) =>
        req.headers.get("authorization") === "Bearer token-operator"
          ? HttpResponse.json({ ok: true })
          : HttpResponse.json(errorBody("unauthorized", "expired"), { status: 401 }),
      ),
      http.post(api("/auth/refresh"), async () => {
        refreshes += 1;
        await new Promise((resolve) => setTimeout(resolve, 20));
        return HttpResponse.json(session("operator"));
      }),
    );
    tokenStore.set("stale");
    await Promise.all([request("/ping", okSchema), request("/ping", okSchema), request("/ping", okSchema)]);
    expect(refreshes).toBe(1);
  });

  it("signals an expired session when the refresh fails", async () => {
    const events: SessionEvent[] = [];
    const unsubscribe = subscribeSession((event) => events.push(event));
    server.use(
      http.get(api("/ping"), () => HttpResponse.json(errorBody("unauthorized", "expired"), { status: 401 })),
      http.post(api("/auth/refresh"), () => HttpResponse.json(errorBody("unauthorized", "no"), { status: 401 })),
    );
    tokenStore.set("stale");
    await expect(request("/ping", okSchema)).rejects.toMatchObject({ status: 401, code: "unauthorized" });
    expect(events).toEqual([{ type: "expired" }]);
    expect(tokenStore.get()).toBeNull();
    unsubscribe();
  });

  it("does not try to refresh when login itself fails with 401", async () => {
    const refresh = vi.fn<() => void>();
    server.use(
      http.post(api("/auth/login"), () => HttpResponse.json(errorBody("invalid_credentials", "bad"), { status: 401 })),
      http.post(api("/auth/refresh"), () => {
        refresh();
        return HttpResponse.json(session());
      }),
    );
    await expect(login("a@b.co", "nope")).rejects.toMatchObject({ code: "invalid_credentials" });
    expect(refresh).not.toHaveBeenCalled();
  });

  it("maps the API error envelope including 422 field errors", async () => {
    server.use(
      http.post(api("/items"), () =>
        HttpResponse.json(errorBody("validation_failed", "invalid", { description: "required" }), { status: 422 }),
      ),
    );
    const error = await request("/items", okSchema, { method: "POST", body: {} }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 422, code: "validation_failed", fields: { description: "required" } });
  });

  it("rejects responses that do not match the schema", async () => {
    server.use(http.get(api("/ping"), () => HttpResponse.json({ nope: 1 })));
    await expect(request("/ping", okSchema)).rejects.toMatchObject({ code: "invalid_response" });
  });

  it("reports network failures with a readable message", async () => {
    server.use(http.get(api("/ping"), () => HttpResponse.error()));
    const error = await request("/ping", okSchema).catch((e: unknown) => e);
    expect(error).toMatchObject({ status: 0, code: "network" });
    expect(describeError(error)).toMatch(/conectar/i);
  });

  it("translates known error codes to Spanish", () => {
    expect(describeError(new ApiError(409, "item_on_loan", "x"))).toMatch(/prestado/i);
    expect(describeError(new ApiError(409, "item_under_repair", "x"))).toMatch(/falla abierta/i);
    expect(describeError(new Error("boom"))).toMatch(/inesperado/i);
  });

  it("returns undefined for 204 responses", async () => {
    server.use(http.delete(api("/items/1"), () => new HttpResponse(null, { status: 204 })));
    await expect(request("/items/1", null, { method: "DELETE" })).resolves.toBeUndefined();
  });

  it("session events reach subscribers", () => {
    const listener = vi.fn<() => void>();
    const unsubscribe = subscribeSession(listener);
    emitSession({ type: "expired" });
    unsubscribe();
    emitSession({ type: "expired" });
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
