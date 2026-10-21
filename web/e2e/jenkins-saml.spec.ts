import { expect, test, type Page, type Cookie } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { call, nextCode } from "./fixtures/jenkins/helpers";
import type { APIState } from "./fixtures/jenkins/helpers";

test.describe.configure({ mode: "serial" });
const fixture = JSON.parse(
  readFileSync(join(process.env.BURROW_SAML_TMP!, "state.json"), "utf8"),
);
const port = process.env.BURROW_SAML_HTTPS_PORT!;
const jenkins = `https://${fixture.jenkinsHost}:${port}`;
let readerCookies: Cookie[] = [];
let adminCookies: Cookie[] = [];
const mfaSecrets = new Map<string, string>();
let capturedResponse = "";
async function identity(page: Page) {
  return page.evaluate(() =>
    fetch("/whoAmI/api/json").then((response) => response.json()),
  );
}
async function signIn(page: Page, username: string) {
  await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
  await expect(page.getByLabel("Username", { exact: true })).toBeVisible();
  await page.getByLabel("Username", { exact: true }).fill(username);
  await page
    .getByLabel("Password", { exact: true })
    .fill("Fixture-user-password-2026");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page).toHaveURL(/\/change-password/);
  await page
    .getByLabel("New password", { exact: true })
    .fill("Fixture-user-changed-password-2026");
  await page
    .getByLabel("Confirm password", { exact: true })
    .fill("Fixture-user-changed-password-2026");
  await page
    .getByRole("button", { name: "Change password", exact: true })
    .click();
  if (fixture.mfaEnabled) {
    await expect(page.getByTestId("mfa-secret")).toBeVisible();
    const secret = (await page.getByTestId("mfa-secret").innerText()).trim();
    mfaSecrets.set(username, secret);
    await page
      .getByLabel("Authenticator code", { exact: true })
      .fill(await nextCode(secret));
    await page
      .getByRole("button", { name: "Verify and continue", exact: true })
      .click();
  }
  await expect(page).toHaveURL(
    new RegExp(`^${jenkins.replaceAll(".", "\\.")}/`),
  );
}

test(`real Jenkins verifies double signatures after forced password change ${fixture.mfaEnabled ? "and MFA" : "with MFA disabled"}`, async ({
  page,
}) => {
  page.on("request", (request) => {
    if (
      request.url() === jenkins + "/securityRealm/finishLogin" &&
      request.method() === "POST"
    )
      capturedResponse = request.postData() || "";
  });
  await signIn(page, "jenkins-reader");
  expect(await identity(page)).toMatchObject({
    authenticated: true,
    anonymous: false,
    name: fixture.readerId,
  });
  expect(capturedResponse).toContain("SAMLResponse=");
  const details = await page.evaluate(async () => {
    const result = await fetch("/me/api/json");
    return result.json();
  });
  expect(details.fullName).toBe("Jenkins reader");
  expect(
    details.property.some(
      (property: { address?: string }) =>
        property.address === "jenkins-reader@example.test",
    ),
  ).toBe(true);
  expect(await page.evaluate(async () => (await fetch("/script")).status)).toBe(
    403,
  );
  readerCookies = (await page.context().cookies()).filter(
    (cookie) => cookie.domain === "sso.burrow.test",
  );
});

test("stable NameID grants only the explicitly configured Jenkins administrator", async ({
  page,
}) => {
  await signIn(page, "jenkins-admin");
  expect(await identity(page)).toMatchObject({
    authenticated: true,
    anonymous: false,
    name: fixture.adminId,
  });
  expect(await page.evaluate(async () => (await fetch("/script")).status)).toBe(
    200,
  );
  adminCookies = await page.context().cookies();
});

test("Jenkins rejects modified assertions and unsolicited replay", async ({
  browser,
}) => {
  const replayState: APIState = { cookies: {} };
  await call(
    fixture.jenkinsHost,
    "/securityRealm/finishLogin",
    "POST",
    capturedResponse,
    replayState,
    true,
  );
  const replayIdentity = await call(
    fixture.jenkinsHost,
    "/whoAmI/api/json",
    "GET",
    undefined,
    replayState,
  );
  expect(replayIdentity.json).toMatchObject({
    anonymous: true,
    name: "anonymous",
  });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await context.addCookies(readerCookies);
    const page = await context.newPage();
    await page.route("**/securityRealm/finishLogin", async (route) => {
      const form = new URLSearchParams(route.request().postData()!);
      const raw = Buffer.from(form.get("SAMLResponse")!, "base64").toString(
        "utf8",
      );
      const changed = raw.replace("Jenkins reader", "Altered reader");
      expect(changed).not.toBe(raw);
      form.set("SAMLResponse", Buffer.from(changed).toString("base64"));
      await route.continue({ postData: form.toString() });
    });
    await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
    await expect(async () =>
      expect((await identity(page)).anonymous).toBe(true),
    ).toPass();
  } finally {
    await context.close();
  }
});

test("tampering with Jenkins Redirect signature is rejected by Burrow", async () => {
  const commenced = await call(
    fixture.jenkinsHost,
    "/securityRealm/commenceLogin?from=%2F",
  );
  expect(commenced.status).toBe(302);
  const target = new URL(commenced.location!);
  expect(target.searchParams.get("SigAlg")).toBe(
    "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256",
  );
  target.searchParams.set("RelayState", "tampered");
  const rejected = await call(
    "sso.burrow.test",
    target.pathname + target.search,
  );
  expect(rejected.status).toBe(400);
  expect(rejected.json.error.code).toBe("invalid_saml_request");
});

function compose(...args: string[]) {
  const project = process.env.BURROW_SAML_PROJECT!;
  if (!/^burrow-saml-[a-f0-9]+$/.test(project))
    throw new Error("Invalid fixture project");
  execFileSync(
    "docker",
    [
      "compose",
      "-p",
      project,
      "-f",
      "web/e2e/fixtures/jenkins/docker-compose.yml",
      ...args,
    ],
    { cwd: join(import.meta.dirname, "../.."), stdio: "pipe" },
  );
}

async function ready() {
  await expect(async () => {
    expect((await call("sso.burrow.test", "/saml/certificate")).status).toBe(
      200,
    );
    expect(
      (await call(fixture.jenkinsHost, "/securityRealm/metadata")).body,
    ).toContain("EntityDescriptor");
  }).toPass({ timeout: 90000 });
}

async function setForceAuthn(page: Page, force: boolean) {
  const status = await page.evaluate(async (force) => {
    const crumb = await (await fetch("/crumbIssuer/api/json")).json();
    const script = `
      import org.jenkinsci.plugins.saml.*
      def j = jenkins.model.Jenkins.get()
      def r = j.securityRealm
      def a = r.advancedConfiguration
      j.setSecurityRealm(new SamlSecurityRealm(r.idpMetadataConfiguration,
        r.displayNameAttributeName, r.groupsAttributeName, r.maximumAuthenticationLifetime,
        r.usernameAttributeName, r.emailAttributeName, r.logoutUrl,
        new SamlAdvancedConfiguration(${force}, a.authnContextClassRef, a.spEntityId, a.nameIdPolicyFormat),
        r.encryptionData, r.usernameCaseConversion, r.binding, r.samlCustomAttributes))
      j.save()
    `;
    return (
      await fetch("/scriptText", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded",
          [crumb.crumbRequestField]: crumb.crumb,
        },
        body: new URLSearchParams({ script }).toString(),
      })
    ).status;
  }, force);
  expect(status).toBe(200);
}

test("portal entry reuses the current Burrow session", async ({ browser }) => {
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await context.addCookies(readerCookies);
    const page = await context.newPage();
    await page.goto(`https://sso.burrow.test:${port}/`);
    const [app] = await Promise.all([
      context.waitForEvent("page", { timeout: 15000 }),
      page
        .getByRole("main")
        .getByRole("link", { name: /Jenkins/ })
        .click(),
    ]);
    await expect(async () =>
      expect(await identity(app)).toMatchObject({
        anonymous: false,
        name: fixture.readerId,
      }),
    ).toPass();
  } finally {
    await context.close();
  }
});

test("Jenkins ForceAuthn requires a new password and current MFA authentication", async ({
  browser,
}) => {
  const adminContext = await browser.newContext({ ignoreHTTPSErrors: true });
  const readerContext = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await adminContext.addCookies(adminCookies);
    const adminPage = await adminContext.newPage();
    await adminPage.goto(jenkins + "/");
    await setForceAuthn(adminPage, true);
    try {
      await readerContext.addCookies(readerCookies);
      const page = await readerContext.newPage();
      await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
      await expect(page.getByLabel("Username", { exact: true })).toBeVisible();
      await page.getByLabel("Username", { exact: true }).fill("jenkins-reader");
      await page
        .getByLabel("Password", { exact: true })
        .fill("Fixture-user-changed-password-2026");
      await page.getByRole("button", { name: "Sign in", exact: true }).click();
      if (fixture.mfaEnabled) {
        await expect(page).toHaveURL(/\/mfa/);
        await page
          .getByLabel("Authenticator code", { exact: true })
          .fill(await nextCode(mfaSecrets.get("jenkins-reader")!));
        await page
          .getByRole("button", { name: "Verify and continue", exact: true })
          .click();
      }
      await expect(page).toHaveURL(
        new RegExp(`^${jenkins.replaceAll(".", "\\.")}/`),
      );
      expect(await identity(page)).toMatchObject({
        anonymous: false,
        name: fixture.readerId,
      });
      readerCookies = (await readerContext.cookies()).filter(
        (cookie) => cookie.domain === "sso.burrow.test",
      );
    } finally {
      await setForceAuthn(adminPage, false);
    }
  } finally {
    await readerContext.close();
    await adminContext.close();
  }
});

test("restarts preserve the IdP certificate, SP configuration and login", async ({
  browser,
}) => {
  const before = await call("sso.burrow.test", "/saml/certificate");
  compose("restart", "burrow", "jenkins");
  await ready();
  expect((await call("sso.burrow.test", "/saml/certificate")).body).toBe(
    before.body,
  );
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await context.addCookies(readerCookies);
    const page = await context.newPage();
    await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
    await expect(async () =>
      expect(await identity(page)).toMatchObject({
        anonymous: false,
        name: fixture.readerId,
      }),
    ).toPass();
  } finally {
    await context.close();
  }
});

test("certificate rotation requires Jenkins trust replacement before new login", async ({
  browser,
}) => {
  const before = await call("sso.burrow.test", "/saml/certificate");
  compose(
    "exec",
    "-T",
    "burrow",
    "burrow",
    "keys-rotate",
    "--protocol",
    "saml",
    "--reason",
    "isolated interoperability test",
  );
  const after = await call("sso.burrow.test", "/saml/certificate");
  expect(after.body).not.toBe(before.body);
  const rejectedContext = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await rejectedContext.addCookies(readerCookies);
    const page = await rejectedContext.newPage();
    await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
    await expect(async () =>
      expect((await identity(page)).anonymous).toBe(true),
    ).toPass();
  } finally {
    await rejectedContext.close();
  }
  const metadata = await call("sso.burrow.test", "/saml/metadata");
  expect(metadata.status).toBe(200);
  writeFileSync(
    join(process.env.BURROW_SAML_TMP!, "public/idp.xml"),
    metadata.body,
  );
  compose("restart", "jenkins");
  await ready();
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await context.addCookies(readerCookies);
    const page = await context.newPage();
    await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
    await expect(async () =>
      expect(await identity(page)).toMatchObject({
        anonymous: false,
        name: fixture.readerId,
      }),
    ).toPass();
  } finally {
    await context.close();
  }
});

test("a Burrow user without app permission receives no Jenkins identity", async ({
  page,
}) => {
  await signIn(page, "jenkins-denied");
  expect((await identity(page)).anonymous).toBe(true);
});

test("revoking the ordinary Burrow role denies new Jenkins sessions", async ({
  browser,
}) => {
  const revoked = await call(
    "sso.burrow.test",
    `/api/v1/roles/${fixture.roleId}`,
    "PUT",
    { permissionIds: [] },
    fixture.api,
  );
  expect(revoked.status).toBe(200);
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  try {
    await context.addCookies(readerCookies);
    const page = await context.newPage();
    await page.goto(jenkins + "/securityRealm/commenceLogin?from=%2F");
    await expect(async () =>
      expect((await identity(page)).anonymous).toBe(true),
    ).toPass();
  } finally {
    await context.close();
  }
});
