import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  timeout: 45_000,
  fullyParallel: false,
  workers: 1,
  use: { baseURL: "http://127.0.0.1:18080", trace: "retain-on-failure" },
  webServer: {
    command: "./e2e/test-server.sh",
    url: "http://127.0.0.1:18080/healthz",
    reuseExistingServer: false,
    timeout: 60_000,
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", use: { ...devices["Desktop Firefox"] } },
    { name: "mobile-chromium", use: { ...devices["Pixel 7"] } },
    { name: "mobile-webkit", use: { ...devices["iPhone 13"] } },
  ],
});
