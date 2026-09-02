import { expect, test } from "@playwright/test";
import { readFile } from "node:fs/promises";
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
  test.setTimeout(70_000);
  const failedResponses: string[] = [];
  const downloadAPIRequests: string[] = [];
  const partialStreamLogs: string[] = [];
  const httpPingRequests: string[] = [];
  let recordingDownload = false;
  page.on("response", (response) => {
    if (response.status() >= 400 && response.url().includes("/api/"))
      failedResponses.push(
        `${response.status()} ${new URL(response.url()).pathname}`,
      );
  });
  page.on("request", (request) => {
    if (request.url().includes("/healthz?ping="))
      httpPingRequests.push(request.url());
    if (recordingDownload && request.url().includes("/api/"))
      downloadAPIRequests.push(new URL(request.url()).pathname);
  });
  page.on("console", (message) => {
    if (
      message.type() === "warning" &&
      message.text().includes("partial stream snapshots")
    )
      partialStreamLogs.push(message.text());
  });
  await login(page);
  await page.getByRole("button", { name: /start test/i }).click();
  await expect(page.getByText("HTTP ping")).toBeVisible();
  await expect(page.locator(".speed-line.download")).toHaveAttribute(
    "points",
    /.+/,
    { timeout: 20_000 },
  );
  expect(httpPingRequests).toHaveLength(8);
  const status = page.locator('section p[aria-live="polite"]');
  try {
    await expect(status).toContainText(/Mbps \/ .*Mbps/, { timeout: 50_000 });
  } catch (error) {
    console.log(
      "speed E2E diagnostics",
      JSON.stringify({ status: await status.textContent(), failedResponses }),
    );
    throw error;
  }
  const partialWarning = page.locator(".measurement-warning");
  if (await partialWarning.count()) {
    await expect(partialWarning).toContainText(/stream.*actual speed/i);
    expect(partialStreamLogs).toHaveLength(1);
  } else {
    expect(partialStreamLogs).toEqual([]);
  }
  const download = page.waitForEvent("download", { timeout: 15_000 });
  recordingDownload = true;
  await page.getByRole("button", { name: /download png/i }).click();
  const artifact = await download;
  expect(artifact.suggestedFilename()).toBe("connection-check.png");
  expect(downloadAPIRequests).toEqual([]);
  const path = await artifact.path();
  expect(path).not.toBeNull();
  const png = await readFile(path!);
  expect([...png.subarray(0, 8)]).toEqual([137, 80, 78, 71, 13, 10, 26, 10]);
  expect(png.readUInt32BE(16)).toBe(1200);
  expect(png.readUInt32BE(20)).toBe(630);
});
