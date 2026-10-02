import { expect, test } from "@playwright/test";
import { spawn } from "node:child_process";
import { resolve } from "node:path";

async function stop(child: ReturnType<typeof spawn>) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  const stopped = new Promise<void>((resolve) =>
    child.once("exit", () => resolve()),
  );
  child.kill();
  await stopped;
}

test("administrator provisions access and ordinary users see only their portal", async ({
  page,
  browser,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/login");
  await expect(page.getByText("Or continue with", { exact: true })).toHaveCount(
    0,
  );
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Initial-admin-password-2026");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL(/change-password/);
  await page
    .getByLabel("Current password", { exact: true })
    .fill("Initial-admin-password-2026");
  await page
    .getByLabel("New password", { exact: true })
    .fill("Changed-admin-password-2026");
  await page
    .getByLabel("Confirm password", { exact: true })
    .fill("Changed-admin-password-2026");
  await page
    .getByRole("button", { name: "Change password", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Your work starts here." }),
  ).not.toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Your workspace, connected." }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Providers", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("link", { name: "Users", exact: true }).click();
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(
    page
      .locator(".ant-drawer .ant-select-selection-item")
      .filter({ hasText: "Viewer" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("switch", { name: "Local password login", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("Username", { exact: true }).fill("alice");
  await page.getByLabel("Name", { exact: true }).fill("Alice");
  await page.getByLabel("Email", { exact: true }).fill("alice@example.test");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Alice-initial-password-2026");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByText("alice@example.test")).toBeVisible();
  const createdUsers = await (await page.request.get("/api/v1/users")).json();
  expect(
    createdUsers.items.find(
      (user: { username: string }) => user.username === "alice",
    ).roleIds,
  ).toEqual([]);
  await page.getByRole("link", { name: "Applications", exact: true }).click();
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await expect(
    page.getByLabel("Allowed providers", { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("switch", { name: "Local password login", exact: true }),
  ).toHaveCount(0);
  await page.getByLabel("Name", { exact: true }).fill("Engineering");
  await page
    .getByLabel("Client ID", { exact: true })
    .fill("engineering-example");
  await page
    .getByLabel("Client secret", { exact: true })
    .fill("engineering-example-secret");
  await page
    .getByLabel("Application login URL", { exact: true })
    .fill("http://localhost:19001/login");
  await page
    .getByLabel("Redirect URIs", { exact: true })
    .fill("http://localhost:19001/callback");
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByText("Save this secret now")).toBeVisible();
  await expect(
    page.getByText("engineering-example-secret", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Close", exact: true }).last().click();
  await page.getByRole("link", { name: "Roles", exact: true }).click();
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Engineers");
  await page
    .getByRole("combobox", { name: "Permissions", exact: true })
    .click();
  await page
    .getByRole("combobox", { name: "Permissions", exact: true })
    .fill("Engineering");
  await page
    .locator(".ant-select-item-option")
    .filter({ hasText: "Engineering · Login" })
    .click();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByText("Engineers", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "Groups", exact: true }).click();
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Engineering team");
  await page.getByLabel("Members", { exact: true }).click();
  await page
    .locator(".ant-select-item-option")
    .filter({ hasText: "Alice" })
    .click();
  await page.getByLabel("Name", { exact: true }).click();
  await page.getByRole("combobox", { name: "Roles", exact: true }).click();
  await page
    .locator(".ant-select-item-option")
    .filter({ hasText: "Engineers" })
    .click();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(
    page.getByText("Engineering team", { exact: true }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Users", exact: true }).click();
  await expect(
    page.getByRole("columnheader", { name: "Groups", exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("row")
      .filter({ hasText: "alice@example.test" })
      .getByText("Engineering team", { exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: "test-results/admin-workspace.png",
    fullPage: true,
  });
  const context = await browser.newContext();
  const ordinary = await context.newPage();
  await ordinary.goto("/login");
  await ordinary.getByLabel("Username", { exact: true }).fill("alice");
  await ordinary
    .getByLabel("Password", { exact: true })
    .fill("Alice-initial-password-2026");
  await ordinary.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(ordinary).toHaveURL(/change-password/);
  await ordinary
    .getByLabel("Current password", { exact: true })
    .fill("Alice-initial-password-2026");
  await ordinary
    .getByLabel("New password", { exact: true })
    .fill("Alice-changed-password-2026");
  await ordinary
    .getByLabel("Confirm password", { exact: true })
    .fill("Alice-changed-password-2026");
  await ordinary
    .getByRole("button", { name: "Change password", exact: true })
    .click();
  await expect(
    ordinary.getByRole("heading", { name: "Engineering", exact: true }),
  ).toBeVisible();
  await expect(
    ordinary.getByRole("link", { name: "Users", exact: true }),
  ).toHaveCount(0);
  expect((await ordinary.request.get("/api/v1/users")).status()).toBe(403);
  await ordinary.getByRole("combobox", { name: "Language" }).click();
  await ordinary.getByText("中文", { exact: true }).click();
  await expect(
    ordinary.getByRole("heading", { name: "我的应用" }),
  ).toBeVisible();
  await ordinary.reload();
  await expect(
    ordinary.getByRole("heading", { name: "我的应用" }),
  ).toBeVisible();
  await ordinary.getByRole("combobox", { name: "外观" }).click();
  await ordinary.getByText("深色", { exact: true }).click();
  await expect(ordinary.locator("html")).toHaveAttribute("data-theme", "dark");
  await ordinary.screenshot({
    path: "test-results/portal-dark-zh.png",
    fullPage: true,
  });
  await ordinary.getByRole("link", { name: "退出登录" }).click();
  await ordinary.getByRole("button", { name: "退出登录", exact: true }).click();
  await expect(ordinary).toHaveURL(/login/);
  expect(errors).toEqual([]);
  await context.close();
});

test("independent Web and SPA OIDC clients share SSO and complete RP logout", async ({
  page,
  request,
}) => {
  test.setTimeout(60000);
  await page.goto("/login");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Changed-admin-password-2026");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your workspace, connected." }),
  ).toBeVisible();
  const csrf = (await (await page.request.get("/api/v1/auth/csrf")).json())
    .token;
  const provision = async (name: string, type: string, port: number) => {
    const response = await page.request.post("/api/v1/applications", {
      headers: { "X-CSRF-Token": csrf },
      data: {
        name,
        clientType: type,
        enabled: true,
        loginUrl: `http://localhost:${port}/login`,
        redirectUris: [`http://localhost:${port}/callback`],
        postLogoutRedirectUris: [`http://localhost:${port}/`],
        origins: type === "spa" ? [`http://localhost:${port}`] : [],
      },
    });
    expect(response.status()).toBe(201);
    return response.json();
  };
  const web = await provision("Web interop", "web", 19001);
  const spa = await provision("SPA interop", "spa", 19002);
  const issuer = process.env.BURROW_E2E_URL || "http://localhost:18080";
  const webChild = spawn(process.env.BURROW_E2E_WEB_BINARY!, [], {
    env: {
      ...process.env,
      OIDC_ISSUER: issuer,
      OIDC_CLIENT_ID: web.clientId,
      OIDC_CLIENT_SECRET: web.clientSecret,
    },
    stdio: "ignore",
  });
  const spaDir = resolve("../examples/spa-client");
  const spaChild = spawn(
    process.execPath,
    [
      resolve(spaDir, "node_modules/vite/bin/vite.js"),
      "--host",
      "127.0.0.1",
      "--port",
      "19002",
      "--strictPort",
    ],
    {
      cwd: spaDir,
      env: {
        ...process.env,
        VITE_OIDC_ISSUER: issuer,
        VITE_OIDC_CLIENT_ID: spa.clientId,
      },
      stdio: "ignore",
    },
  );
  try {
    await expect
      .poll(async () => {
        try {
          return (await request.get("http://localhost:19001/")).status();
        } catch {
          return 0;
        }
      })
      .toBe(200);
    await expect
      .poll(async () => {
        try {
          return (await request.get("http://localhost:19002/")).status();
        } catch {
          return 0;
        }
      })
      .toBe(200);
    const logout = await page.request.post("/api/v1/auth/logout", {
      headers: { "X-CSRF-Token": csrf },
    });
    expect(logout.status()).toBe(200);
    await page.goto("http://localhost:19001/login");
    await expect(page).toHaveURL(/localhost:18080\/login\?requestId=/);
    await expect(page.getByText("Web interop", { exact: true })).toBeVisible();
    await page.getByLabel("Username", { exact: true }).fill("admin");
    await page
      .getByLabel("Password", { exact: true })
      .fill("Changed-admin-password-2026");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page).toHaveURL(/localhost:19001\/callback/);
    await expect(page.locator("body")).toContainText(
      "OIDC code + PKCE verified with coreos/go-oidc",
    );
    await page.goto("http://localhost:19002/");
    await page.getByRole("button", { name: "Sign in with Burrow" }).click();
    await expect(page.locator("#result")).toContainText(
      '"preferred_username": "admin"',
    );
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(
      page.getByRole("heading", { name: "Sign out of Burrow?" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await expect(page).toHaveURL(/localhost:19002/);
    expect((await page.request.get(issuer + "/api/v1/me")).status()).toBe(401);
  } finally {
    await Promise.all([stop(webChild), stop(spaChild)]);
  }
});

test("administrator enables PKCE compatibility for an independent Web client", async ({
  page,
  request,
}) => {
  test.setTimeout(60000);
  await page.goto("/login");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Changed-admin-password-2026");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your workspace, connected." }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Applications", exact: true }).click();
  await page.getByRole("button", { name: "Create", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill("Legacy Web interop");
  await page
    .getByLabel("Application login URL", { exact: true })
    .fill("http://localhost:19001/login");
  await page
    .getByLabel("Redirect URIs", { exact: true })
    .fill("http://localhost:19001/callback");
  const compatibility = page.getByRole("switch", {
    name: "Allow login without PKCE",
    exact: true,
  });
  await expect(compatibility).not.toBeChecked();
  await page.getByLabel("Client type", { exact: true }).click();
  await page
    .locator(".ant-select-item-option")
    .filter({ hasText: "Browser SPA" })
    .click();
  await expect(compatibility).not.toBeVisible();
  await page.getByLabel("Client type", { exact: true }).click();
  await page
    .locator(".ant-select-item-option")
    .filter({ hasText: "Server-side web" })
    .click();
  await compatibility.check();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await expect(page.getByText("Save this secret now")).toBeVisible();
  const secret = (await page.locator(".secret-value").innerText()).trim();
  await page.getByRole("button", { name: "Close", exact: true }).last().click();
  const apps = await (await page.request.get("/api/v1/applications")).json();
  const app = apps.items.find(
    (row: { name: string }) => row.name === "Legacy Web interop",
  );
  expect(app.allowWithoutPkce).toBe(true);
  const issuer = process.env.BURROW_E2E_URL || "http://localhost:18080";
  const child = spawn(process.env.BURROW_E2E_WEB_BINARY!, [], {
    env: {
      ...process.env,
      OIDC_ISSUER: issuer,
      OIDC_CLIENT_ID: app.clientId,
      OIDC_CLIENT_SECRET: secret,
      OIDC_USE_PKCE: "false",
    },
    stdio: "ignore",
  });
  try {
    await expect
      .poll(async () => {
        try {
          return (await request.get("http://localhost:19001/")).status();
        } catch {
          return 0;
        }
      })
      .toBe(200);
    const authorization = page.waitForRequest((r) =>
      r.url().startsWith(issuer + "/oidc/authorize?"),
    );
    await page.goto("http://localhost:19001/login");
    const params = new URL((await authorization).url()).searchParams;
    expect(params.has("code_challenge")).toBe(false);
    expect(params.has("code_challenge_method")).toBe(false);
    await expect(page).toHaveURL(/localhost:19001\/callback/);
    await expect(page.locator("body")).toContainText(
      "OIDC code without PKCE verified with coreos/go-oidc",
    );
    await expect(page.locator("body")).toContainText(
      '"preferred_username":"admin"',
    );
  } finally {
    await stop(child);
  }
});
