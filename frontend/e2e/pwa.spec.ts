import { expect, test, type Page } from "@playwright/test";
import { login } from "./helpers";

// This file needs the real service worker; every other spec blocks it.
test.use({ serviceWorkers: "allow" });

async function waitForControl(page: Page) {
  await page.evaluate(() => navigator.serviceWorker.ready.then(() => undefined));
  // clientsClaim lets the worker control the page without a reload; wait for it.
  await page.waitForFunction(() => navigator.serviceWorker.controller !== null, undefined, { timeout: 15_000 });
}

async function cachedUrls(page: Page): Promise<string[]> {
  return page.evaluate(async () => {
    const urls: string[] = [];
    for (const name of await caches.keys()) {
      const cache = await caches.open(name);
      urls.push(...(await cache.keys()).map((request) => new URL(request.url).pathname));
    }
    return urls;
  });
}

test("serves a valid manifest and registers a service worker that precaches the shell only", async ({ page, request }) => {
  await page.goto("/login");
  await waitForControl(page);

  const manifestHref = await page.locator('link[rel="manifest"]').getAttribute("href");
  expect(manifestHref).toBeTruthy();
  const manifest = (await (await request.get(manifestHref as string)).json()) as {
    name: string;
    display: string;
    start_url: string;
    icons: { sizes: string; purpose?: string }[];
  };
  expect(manifest.name).toBe("Inventario Iskra");
  expect(manifest.display).toBe("standalone");
  expect(manifest.icons.some((icon) => icon.purpose === "maskable")).toBe(true);
  expect(manifest.icons.some((icon) => icon.sizes === "512x512")).toBe(true);

  const urls = await cachedUrls(page);
  expect(urls).toContain("/index.html");
  expect(urls.some((url) => url.startsWith("/assets/") && url.endsWith(".js"))).toBe(true);
  expect(urls.filter((url) => url.startsWith("/api"))).toEqual([]);
});

test("API and auth responses never end up in a cache", async ({ page }) => {
  await login(page, "operator");
  await waitForControl(page);
  // Generate API traffic the worker could intercept: dashboard, catalogs, items.
  await page.goto("/materiales");
  await expect(page.getByRole("heading", { name: "Materiales" })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Materiales" })).toBeVisible();

  const urls = await cachedUrls(page);
  expect(urls.filter((url) => url.includes("/api"))).toEqual([]);

  // Every API request reaches the network, even with the worker in control.
  let hits = 0;
  page.on("request", (req) => {
    if (new URL(req.url()).pathname === "/api/v1/dashboard/summary") hits += 1;
  });
  // Wait for each page: every full load rotates the refresh cookie, so do not abort one mid-flight.
  await page.goto("/inicio");
  await expect(page.getByRole("heading", { name: "Inicio" })).toBeVisible();
  await page.goto("/materiales");
  await expect(page.getByRole("heading", { name: "Materiales" })).toBeVisible();
  await page.goto("/inicio");
  await expect(page.getByRole("heading", { name: "Inicio" })).toBeVisible();
  expect(hits).toBeGreaterThanOrEqual(2);
});

test("offline: the shell loads from the cache and says there is no connection", async ({ page, context }) => {
  await login(page, "operator");
  await waitForControl(page);
  await page.reload(); // make sure the worker finished precaching before going offline
  // Let the refresh of this load finish: aborting one mid-flight loses the rotated cookie.
  await expect(page.getByRole("heading", { name: "Inicio" })).toBeVisible();

  await context.setOffline(true);
  await page.reload();
  await expect(page.getByText(/Sin conexión/).first()).toBeVisible();
  // The session cannot be verified offline, but the user is not logged out: they can retry.
  await expect(page.getByText(/No se pudo conectar para verificar tu sesión/)).toBeVisible();

  // Coming back online re-verifies the session on its own.
  await context.setOffline(false);
  await expect(page.getByRole("heading", { name: "Inicio" })).toBeVisible();
  await expect(page.getByText(/Sin conexión/)).toHaveCount(0);
});

test("offline while logged in: API failures use the existing error states", async ({ page, context }) => {
  await login(page, "operator");
  await waitForControl(page);
  await context.setOffline(true);
  await expect(page.getByText(/Sin conexión/).first()).toBeVisible();
  await page.getByRole("link", { name: "Materiales" }).first().click();
  // Queries retry twice with backoff before surfacing the error.
  await expect(page.getByRole("alert")).toContainText("No se pudo conectar con el servidor", { timeout: 20_000 });
  await context.setOffline(false);
});
