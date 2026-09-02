import { expect, test } from "@playwright/test";
async function login(page: import("@playwright/test").Page) {
  await page.goto("/");
  await page.getByLabel(/shared password/i).fill("pcc-e2e-only-password");
  await page.getByRole("button", { name: /sign in/i }).click();
  await expect(page.getByRole("button", { name: /start test/i })).toBeVisible();
}
test("password login, stability controls, history deletion and PNG", async ({
  page,
}) => {
  await login(page);
  await page.getByRole("button", { name: "Stability" }).click();
  await page.getByRole("button", { name: /start test/i }).click();
  await expect(page.getByText(/ms · jitter|Measurement paused/)).toBeVisible();
  await page.getByRole("button", { name: "1h" }).click();
  await page.getByRole("button", { name: /cancel/i }).click();
  await page.getByRole("button", { name: "History" }).click();
  page.once("dialog", (d) => d.accept());
  await page.getByRole("button", { name: /delete all/i }).click();
});
test("speed result includes upload and PNG download", async ({ page }) => {
  await login(page);
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: /start test/i }).click();
  await expect(page.getByText(/Mbps \/ .*Mbps/)).toBeVisible({
    timeout: 35_000,
  });
  await page.getByRole("button", { name: /download png/i }).click();
  await expect(await download).toBeTruthy();
});
