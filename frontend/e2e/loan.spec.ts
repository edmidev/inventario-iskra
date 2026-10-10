import { expect, test } from "@playwright/test";
import { SEED } from "./env";
import { choose, login, openItem } from "./helpers";

test("operator lends an asset, sees it unavailable, then returns it", async ({ page }) => {
  await login(page, "operator");
  await openItem(page, SEED.loanAsset.code);
  await expect(page.getByText("Disponible")).toBeVisible();

  await page.getByRole("button", { name: "Prestar", exact: true }).click();
  const lend = page.getByRole("dialog", { name: "Prestar material" });
  await choose(page, /Prestar a/, new RegExp(SEED.borrower));
  await lend.getByRole("button", { name: "Registrar préstamo" }).click();
  await expect(page.getByText(`${SEED.loanAsset.code} prestado a ${SEED.borrower}.`, { exact: true })).toBeVisible();

  // Now unavailable: the badge, the custody card and the loans page all say so.
  await expect(page.getByText(`Prestado a ${SEED.borrower}`).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Prestar", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Trasladar" })).toBeDisabled();
  await page.goto("/prestamos");
  await expect(page.getByRole("list", { name: "Préstamos activos" })).toContainText(SEED.loanAsset.code);

  await page.getByRole("button", { name: `Devolver ${SEED.loanAsset.code}` }).click();
  await page.getByRole("dialog", { name: "Registrar devolución" }).getByRole("button", { name: "Registrar devolución" }).click();
  await expect(page.getByText("No hay préstamos activos")).toBeVisible();

  await openItem(page, SEED.loanAsset.code);
  await expect(page.getByText("Disponible")).toBeVisible();
  await expect(page.getByRole("button", { name: "Prestar", exact: true })).toBeEnabled();
});
