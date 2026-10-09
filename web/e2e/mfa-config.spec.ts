import { expect, test } from "@playwright/test";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";

test("global MFA configuration preserves password changes and one-click logout", async ({
  page,
  request,
}) => {
  const binary = process.env.BURROW_E2E_BINARY!;
  const directory = mkdtempSync(join(tmpdir(), "burrow-mfa-config-"));
  const listener = createServer();
  await new Promise<void>((resolve) =>
    listener.listen(0, "127.0.0.1", resolve),
  );
  const port = (listener.address() as { port: number }).port;
  await new Promise<void>((resolve, reject) =>
    listener.close((error) => (error ? reject(error) : resolve())),
  );
  const origin = `http://127.0.0.1:${port}`;
  const env = {
    ...process.env,
    BURROW_DB_DSN: join(directory, "test.db"),
    BURROW_LISTEN_ADDR: `127.0.0.1:${port}`,
    BURROW_ISSUER: origin,
    BURROW_MFA_ENABLED: "false",
  };
  let server: ReturnType<typeof spawn> | undefined;
  const stop = async () => {
    if (!server || server.exitCode !== null || server.signalCode !== null)
      return;
    const stopped = new Promise<void>((resolve) =>
      server!.once("exit", () => resolve()),
    );
    server.kill();
    await stopped;
  };
  const start = async (enabled: boolean) => {
    await stop();
    server = spawn(binary, ["serve"], {
      env: { ...env, BURROW_MFA_ENABLED: String(enabled) },
      stdio: "ignore",
    });
    await expect(async () => {
      expect((await request.get(`${origin}/readyz`)).status()).toBe(200);
    }).toPass({ timeout: 10000 });
  };
  const login = async (password: string) => {
    await page.goto(`${origin}/login`);
    await page.getByLabel("Username", { exact: true }).fill("admin");
    await page.getByLabel("Password", { exact: true }).fill(password);
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
  };
  try {
    execFileSync(binary, ["migrate"], { env, stdio: "ignore" });
    execFileSync(binary, ["seed"], { env, stdio: "ignore" });
    await start(false);
    await login("Initial-admin-password-2026");
    await expect(page).toHaveURL(/change-password/);
    expect((await page.request.get(`${origin}/api/v1/me`)).status()).toBe(401);
    await page
      .getByLabel("New password", { exact: true })
      .fill("Changed-admin-password-2026");
    await page
      .getByLabel("Confirm password", { exact: true })
      .fill("Changed-admin-password-2026");
    await page
      .getByRole("button", { name: "Change password", exact: true })
      .click();
    await expect(page).toHaveURL(`${origin}/`);
    await page.locator(".user-block").click();
    await expect(
      page.getByText("MFA verification is disabled for all accounts", {
        exact: true,
      }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page).toHaveURL(`${origin}/login`);
    expect((await page.request.get(`${origin}/api/v1/me`)).status()).toBe(401);
    await expect(page.locator(".auth-story .brand")).toHaveText("burrow");
    await login("Changed-admin-password-2026");
    await expect(page).toHaveURL(`${origin}/`);
    await start(true);
    await page.reload();
    await expect(page).toHaveURL(`${origin}/login`);
    await login("Changed-admin-password-2026");
    await expect(page).toHaveURL(`${origin}/mfa`);
    await expect(page.getByTestId("mfa-secret")).toBeVisible();
    await start(false);
    await page.goto(`${origin}/login`);
    await expect(page.getByLabel("Username", { exact: true })).toBeVisible();
    await login("Changed-admin-password-2026");
    await expect(page).toHaveURL(`${origin}/`);
  } finally {
    await stop();
    rmSync(directory, { recursive: true, force: true });
  }
});
