import { expect, test } from "@playwright/test";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { sign } from "node:crypto";
import { deflateRawSync } from "node:zlib";

test("SAML application management and typed ForceAuthn continuation", async ({
  page,
  request,
}) => {
  const binary = process.env.BURROW_E2E_BINARY!;
  const directory = mkdtempSync(join(tmpdir(), "burrow-saml-ui-"));
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
  try {
    execFileSync(binary, ["migrate"], { env, stdio: "ignore" });
    execFileSync(binary, ["seed"], { env, stdio: "ignore" });
    server = spawn(binary, ["serve"], { env, stdio: "ignore" });
    await expect(async () =>
      expect((await request.get(`${origin}/readyz`)).status()).toBe(200),
    ).toPass({ timeout: 10000 });
    execFileSync(
      "openssl",
      [
        "req",
        "-x509",
        "-newkey",
        "rsa:2048",
        "-nodes",
        "-keyout",
        join(directory, "sp.key"),
        "-out",
        join(directory, "sp.crt"),
        "-days",
        "1",
        "-subj",
        "/CN=burrow-test-sp",
      ],
      { stdio: "ignore" },
    );
    const cert = readFileSync(join(directory, "sp.crt"), "utf8").replace(
      /-----[^-]+-----|\s/g,
      "",
    );
    const entity = "https://jenkins.example.test/securityRealm/finishLogin";
    const metadata = `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="${entity}"><SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"><KeyDescriptor use="signing"><ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:X509Data><ds:X509Certificate>${cert}</ds:X509Certificate></ds:X509Data></ds:KeyInfo></KeyDescriptor><AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="${entity}" index="0"/></SPSSODescriptor></EntityDescriptor>`;
    await page.goto(`${origin}/login`);
    await page.getByLabel("Username", { exact: true }).fill("admin");
    await page
      .getByLabel("Password", { exact: true })
      .fill("Initial-admin-password-2026");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page
      .getByLabel("New password", { exact: true })
      .fill("Changed-admin-password-2026");
    await page
      .getByLabel("Confirm password", { exact: true })
      .fill("Changed-admin-password-2026");
    await page
      .getByRole("button", { name: "Change password", exact: true })
      .click();
    await page.getByRole("link", { name: "Applications", exact: true }).click();
    await page.getByRole("button", { name: "Create", exact: true }).click();
    await page.getByLabel("Protocol", { exact: true }).click();
    await page.getByText("SAML 2.0", { exact: true }).click();
    await expect(page.getByLabel("Client ID", { exact: true })).toHaveCount(0);
    await expect(page.getByLabel("Client secret", { exact: true })).toHaveCount(
      0,
    );
    await page.getByLabel("Name", { exact: true }).fill("Jenkins browser test");
    await page
      .getByLabel("Application login URL", { exact: true })
      .fill("https://jenkins.example.test/");
    await page.getByLabel("SP metadata XML", { exact: true }).fill(metadata);
    const saved = page.waitForResponse(
      (response) =>
        response.url() === `${origin}/api/v1/applications` &&
        response.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    const result = await saved;
    expect(result.status()).toBe(201);
    const payload = result.request().postDataJSON();
    expect(payload.protocol).toBe("saml");
    expect(payload).not.toHaveProperty("clientType");
    expect(payload).not.toHaveProperty("clientSecret");
    expect(payload.saml).toEqual({ metadataXml: metadata });
    await expect(
      page.getByText("Jenkins browser test", { exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Edit", exact: true }).click();
    await expect(page.getByLabel("Protocol", { exact: true })).toBeDisabled();
    await expect(page.getByText(entity, { exact: true }).first()).toBeVisible();
    await page
      .getByRole("button", { name: "Close", exact: true })
      .first()
      .click();
    const raw = `<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_browser-request" Version="2.0" IssueInstant="${new Date().toISOString()}" Destination="${origin}/saml/sso" AssertionConsumerServiceURL="${entity}" ForceAuthn="true"><saml:Issuer>${entity}</saml:Issuer></samlp:AuthnRequest>`;
    const algorithm = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256";
    const signed = `SAMLRequest=${encodeURIComponent(deflateRawSync(raw).toString("base64"))}&RelayState=opaque&SigAlg=${encodeURIComponent(algorithm)}`;
    const signature = sign(
      "RSA-SHA256",
      Buffer.from(signed),
      readFileSync(join(directory, "sp.key")),
    ).toString("base64");
    await page.route(entity, (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<p>Test SP received POST</p>",
      }),
    );
    await page.goto(
      `${origin}/saml/sso?${signed}&Signature=${encodeURIComponent(signature)}`,
    );
    await expect(page).toHaveURL(/\/login\?requestType=saml/);
    await expect(
      page.getByText("Jenkins browser test", { exact: true }),
    ).toBeVisible();
    const reference = new URL(page.url()).searchParams.get("requestId");
    expect(
      (
        await page.request.get(
          `${origin}/api/v1/auth/context?requestId=${reference}&requestType=oidc`,
        )
      ).status(),
    ).toBe(400);
    await page.getByLabel("Username", { exact: true }).fill("admin");
    await page
      .getByLabel("Password", { exact: true })
      .fill("Changed-admin-password-2026");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await expect(page.getByText("Test SP received POST")).toBeVisible();
  } finally {
    if (server && server.exitCode === null && server.signalCode === null) {
      const stopped = new Promise<void>((resolve) =>
        server!.once("exit", () => resolve()),
      );
      server.kill();
      await stopped;
    }
    rmSync(directory, { recursive: true, force: true });
  }
});
