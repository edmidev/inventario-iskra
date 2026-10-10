import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { ADMIN, API_URL, SEED } from "./env";

// Opt-in visual pass: E2E_SCREENSHOTS=<dir> npm run e2e -- visual
// Saves light/dark mobile screenshots (and two desktop ones) and an automatic layout audit
// (horizontal overflow, tap targets under 44px). Skipped in normal runs.
const outDir = process.env.E2E_SCREENSHOTS;
test.skip(!outDir, "set E2E_SCREENSHOTS to a directory to capture screenshots");
test.setTimeout(240_000);

async function api(token: string, route: string, body: unknown): Promise<void> {
  const response = await fetch(`${API_URL}/api/v1${route}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error(`${route} -> ${response.status} ${await response.text()}`);
}

async function lookup<T>(token: string, route: string): Promise<T> {
  const response = await fetch(`${API_URL}/api/v1${route}`, { headers: { Authorization: `Bearer ${token}` } });
  return (await response.json()) as T;
}

/** Puts the data in states worth looking at: an overdue loan, low stock, an open failure. */
async function prepareData(): Promise<void> {
  const session = (await (
    await fetch(`${API_URL}/api/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: ADMIN.email, password: ADMIN.password }),
    })
  ).json()) as { access_token: string };
  const token = session.access_token;
  const items = await lookup<{ data: { id: string; code: string }[] }>(token, "/items?limit=100");
  const id = (code: string) => items.data.find((i) => i.code === code)?.id as string;
  const locations = await lookup<{ data: { id: number; name: string }[] }>(token, "/locations");
  const borrower = locations.data.find((l) => l.name === SEED.borrower)?.id as number;

  const day = 24 * 3600 * 1000;
  await api(token, `/items/${id(SEED.qrAsset.code)}/assign`, { assignee_location_id: borrower, due_at: new Date(Date.now() + day).toISOString() }).catch(() => undefined);
  await api(token, `/items/${id(SEED.consumable.code)}/movements`, { type: "adjust", qty: 2, note: "Conteo visual" }).catch(() => undefined);
  await api(token, `/items/${id(SEED.failureAsset.code)}/maintenance`, {
    kind: "corrective",
    description: "El motor se calienta y huele a quemado después de pocos minutos de uso continuo",
    item_status: "inoperative",
    destination: "Taller externo con nombre bastante largo para probar el recorte",
  }).catch(() => undefined);
}


const settle = async (page: Page) => {
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(250);
};

async function closeDialog(page: Page): Promise<void> {
  await page.getByRole("dialog").getByRole("button", { name: "Cerrar" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
}

const audit: Record<string, unknown> = {};

async function layoutAudit(page: Page, label: string): Promise<void> {
  const result = await page.evaluate(() => {
    const overflowX = document.documentElement.scrollWidth - window.innerWidth;
    const small: string[] = [];
    const candidates = document.querySelectorAll("a[href], button, input:not([type=hidden]), select, [role=combobox], [role=tab], label:has(input)");
    for (const el of candidates) {
      const box = (el as HTMLElement).getBoundingClientRect();
      if (box.width === 0 || box.height === 0) continue;
      const style = getComputedStyle(el);
      if (style.visibility === "hidden" || style.display === "none") continue;
      if ((el as HTMLElement).classList.contains("sr-only")) continue;
      if (box.height < 43.5 || box.width < 43.5) {
        const name = (el.getAttribute("aria-label") ?? el.textContent ?? "").trim().slice(0, 40);
        small.push(`${el.tagName.toLowerCase()} "${name}" ${Math.round(box.width)}x${Math.round(box.height)}`);
      }
    }
    return { overflowX, small };
  });
  audit[label] = result;
}

test("capture screenshots", async ({ page }) => {
  mkdirSync(outDir as string, { recursive: true });
  await prepareData();
  const shot = async (scheme: "light" | "dark", name: string, size?: { width: number; height: number }) => {
    await page.screenshot({ path: path.join(outDir as string, `${name}-${scheme}${size ? "-desktop" : ""}.png`), fullPage: !size });
  };

  for (const scheme of ["light", "dark"] as const) {
    await page.emulateMedia({ colorScheme: scheme, reducedMotion: "reduce" });
    await page.setViewportSize({ width: 390, height: 844 });

    const run = async (name: string, fn: () => Promise<void>) => {
      await fn();
      await settle(page);
      await layoutAudit(page, `${name}-${scheme}`);
      await shot(scheme, name);
    };

    // Anonymous login page first, then sign in.
    await page.context().clearCookies();
    await run("01-login", async () => {
      await page.goto("/login");
    });
    await page.getByLabel("Correo").fill(ADMIN.email);
    await page.getByLabel("Contraseña").fill(ADMIN.password);
    await page.getByRole("button", { name: "Entrar" }).click();
    await expect(page.getByRole("navigation", { name: "Navegación principal" }).first()).toBeVisible();
    await run("02-dashboard", async () => {
      await page.goto("/inicio");
      await expect(page.getByRole("heading", { name: "Inicio" })).toBeVisible();
    });
    await run("03-items", async () => {
      await page.goto("/materiales");
      await expect(page.getByRole("list").first()).toBeVisible();
    });
    await run("04-item-detail", async () => {
      await page.goto(`/materiales/codigo/${SEED.readOnlyAsset.code}`);
      await expect(page.getByRole("heading", { name: SEED.readOnlyAsset.code })).toBeVisible();
    });
    await run("05-item-action-sheet", async () => {
      await page.getByRole("button", { name: "Prestar", exact: true }).click();
      await expect(page.getByRole("dialog", { name: "Prestar material" })).toBeVisible();
    });
    await closeDialog(page);
    await run("06-item-label-sheet", async () => {
      await page.getByRole("button", { name: "Etiqueta QR" }).click();
      await expect(page.getByRole("dialog", { name: "Etiqueta QR" })).toBeVisible();
    });
    await closeDialog(page);
    await run("07-consumable-detail", async () => {
      await page.goto(`/materiales/codigo/${SEED.consumable.code}`);
      await expect(page.getByRole("heading", { name: SEED.consumable.code })).toBeVisible();
    });
    await run("08-stock-alerts", async () => {
      await page.goto("/stock-bajo");
      await expect(page.getByRole("heading", { name: "Stock bajo" })).toBeVisible();
    });
    await run("09-loans", async () => {
      await page.goto("/prestamos");
      await expect(page.getByRole("heading", { name: "Préstamos" })).toBeVisible();
    });
    await run("10-maintenance", async () => {
      await page.goto("/mantenimiento");
      await expect(page.getByRole("heading", { name: "Mantenimiento" })).toBeVisible();
    });
    await run("11-scan", async () => {
      await page.goto("/escanear");
      await expect(page.getByRole("heading", { name: "Escanear" })).toBeVisible();
    });
    await run("12-more", async () => {
      await page.goto("/mas");
      await expect(page.getByRole("navigation", { name: "Administración" })).toBeVisible();
    });
    await run("13-admin-users", async () => {
      await page.goto("/admin/usuarios");
      await expect(page.getByRole("list", { name: "Usuarios" })).toBeVisible();
    });
    await run("14-admin-categories", async () => {
      await page.goto("/admin/categorias");
      await expect(page.getByRole("list", { name: "Categorías" })).toBeVisible();
    });
    await run("15-admin-locations", async () => {
      await page.goto("/admin/ubicaciones");
      await expect(page.getByRole("list", { name: "Ubicaciones" })).toBeVisible();
    });
    await run("16-account", async () => {
      await page.goto("/cuenta");
      await expect(page.getByRole("heading", { name: "Mi cuenta" })).toBeVisible();
    });
    await run("17-labels", async () => {
      await page.goto("/materiales/etiquetas");
      await page.getByRole("button", { name: "Seleccionar visibles" }).click();
    });
    await run("18-user-form", async () => {
      await page.goto("/admin/usuarios");
      await page.getByRole("button", { name: "Nuevo" }).click();
      await expect(page.getByRole("dialog", { name: "Nuevo usuario" })).toBeVisible();
    });
    await closeDialog(page);

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/inicio");
    await settle(page);
    await shot(scheme, "02-dashboard", { width: 1280, height: 800 });
    await page.goto("/materiales");
    await expect(page.getByRole("list").first()).toBeVisible();
    await settle(page);
    await shot(scheme, "03-items", { width: 1280, height: 800 });
    await page.goto("/mas");
    await page.getByRole("region", { name: "Cuenta" }).getByRole("button", { name: /Cerrar sesión/ }).click();
  }
  writeFileSync(path.join(outDir as string, "audit.json"), JSON.stringify(audit, null, 2));
});
