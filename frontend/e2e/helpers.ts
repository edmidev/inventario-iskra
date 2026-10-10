import { expect, type Page } from "@playwright/test";
import { ADMIN, OPERATOR, VIEWER } from "./env";

export const users = { admin: ADMIN, operator: OPERATOR, viewer: VIEWER } as const;

export async function login(page: Page, role: keyof typeof users): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Correo").fill(users[role].email);
  await page.getByLabel("Contraseña").fill(users[role].password);
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("heading", { name: "Inicio" })).toBeVisible();
}

/** Opens an item's detail page by searching for its code in the materials list. */
export async function openItem(page: Page, code: string): Promise<void> {
  await page.goto("/materiales");
  await page.getByLabel("Buscar materiales").fill(code);
  await page.getByRole("link", { name: new RegExp(code) }).click();
  await expect(page.getByRole("heading", { name: code })).toBeVisible();
}

/** Picks an option in a Radix select by the trigger's accessible name. */
export async function choose(page: Page, label: RegExp | string, option: RegExp | string): Promise<void> {
  await page.getByRole("combobox", { name: label }).click();
  await page.getByRole("option", { name: option }).click();
}
