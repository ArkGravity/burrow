import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { call, nextCode, type APIState } from "./helpers";

const directory = process.env.BURROW_SAML_TMP!;
const state: APIState = { cookies: {} };
const sso = "sso.burrow.test";
const jenkins = process.env.BURROW_SAML_JENKINS_HOST || "jenkins.burrow.test";
const port = process.env.BURROW_SAML_HTTPS_PORT;
const mfaEnabled = process.env.BURROW_SAML_MFA_ENABLED !== "false";
async function api(path: string, method = "GET", body?: unknown) {
  const result = await call(sso, "/api/v1" + path, method, body, state);
  if (result.status < 200 || result.status >= 300)
    throw new Error(`Fixture ${method} ${path} failed (${result.status})`);
  return result.json;
}
state.csrf = (await api("/auth/csrf")).token;
await api("/auth/login", "POST", {
  username: "admin",
  password: "Fixture-initial-password-2026",
});
await api("/auth/login/password", "POST", {
  password: "Fixture-admin-password-2026",
});
if (mfaEnabled) {
  const binding = await api("/auth/login/bind", "POST", {});
  await api("/auth/login/verify", "POST", {
    code: await nextCode(binding.secret),
  });
}
const admin = await api("/users", "POST", {
  username: "jenkins-admin",
  name: "Jenkins administrator",
  email: "jenkins-admin@example.test",
  password: "Fixture-user-password-2026",
});
const reader = await api("/users", "POST", {
  username: "jenkins-reader",
  name: "Jenkins reader",
  email: "jenkins-reader@example.test",
  password: "Fixture-user-password-2026",
});
const denied = await api("/users", "POST", {
  username: "jenkins-denied",
  name: "Jenkins denied",
  email: "jenkins-denied@example.test",
  password: "Fixture-user-password-2026",
});
const metadata = await call(sso, "/saml/metadata");
if (metadata.status !== 200)
  throw new Error("Fixture IdP metadata unavailable");
writeFileSync(join(directory, "public", "idp.xml"), metadata.body);
writeFileSync(
  join(directory, "public", "users.json"),
  JSON.stringify({ adminId: admin.id, readerId: reader.id }),
);
execFileSync(
  "docker",
  [
    "compose",
    "-p",
    process.env.BURROW_SAML_PROJECT!,
    "-f",
    "web/e2e/fixtures/jenkins/docker-compose.yml",
    "up",
    "-d",
    "jenkins",
  ],
  { stdio: "inherit" },
);
let spMetadata = "";
for (let attempt = 0; attempt < 120; attempt++) {
  try {
    const result = await call(jenkins, "/securityRealm/metadata");
    if (result.status === 200 && result.body.includes("EntityDescriptor")) {
      spMetadata = result.body;
      break;
    }
  } catch {}
  await new Promise((resolve) => setTimeout(resolve, 1000));
}
if (!spMetadata)
  throw new Error("Fixture Jenkins metadata did not become ready");
const app = await api("/applications", "POST", {
  name: "Jenkins",
  protocol: "saml",
  loginUrl: `https://${jenkins}:${port}/securityRealm/commenceLogin?from=%2F`,
  saml: { metadataXml: spMetadata },
});
const role = await api("/roles", "POST", {
  name: "Jenkins login",
  permissionIds: [`app:${app.id}:login`],
});
await api(`/users/${admin.id}`, "PUT", { roleIds: [role.id] });
await api(`/users/${reader.id}`, "PUT", { roleIds: [role.id] });
writeFileSync(
  join(directory, "state.json"),
  JSON.stringify({
    api: state,
    appId: app.id,
    roleId: role.id,
    adminId: admin.id,
    readerId: reader.id,
    deniedId: denied.id,
    jenkinsHost: jenkins,
    mfaEnabled,
  }),
  { mode: 0o600 },
);
console.log("Isolated Jenkins SAML fixture configured");
