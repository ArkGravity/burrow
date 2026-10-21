import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "jenkins-saml.spec.ts",
  workers: 1,
  timeout: 120000,
  expect: { timeout: 15000 },
  use: {
    ...devices["Desktop Chrome"],
    ignoreHTTPSErrors: true,
    trace: "off",
    screenshot: "off",
    launchOptions: {
      args: [
        "--host-resolver-rules=MAP *.burrow.test 127.0.0.1, MAP *.burrow-sp.test 127.0.0.1, EXCLUDE localhost",
      ],
    },
  },
  reporter: [["list"]],
});
