import { expect, it } from "vitest";
import { applicationPayload } from "./applications";

it("submits only metadata and common fields for SAML", () => {
  expect(
    applicationPayload({
      name: "Jenkins",
      protocol: "saml",
      clientId: "internal",
      clientType: "web",
      redirectUris: ["https://old.test"],
      clientSecret: "old",
      allowWithoutPkce: true,
      saml: { metadataXml: "<metadata/>", entityId: "readonly" },
    }),
  ).toEqual({
    name: "Jenkins",
    protocol: "saml",
    saml: { metadataXml: "<metadata/>" },
  });
});
it("removes SAML input when switching back to OIDC", () => {
  expect(
    applicationPayload({
      name: "RP",
      protocol: "oidc",
      clientType: "web",
      saml: { metadataXml: "old" },
    }),
  ).toEqual({ name: "RP", protocol: "oidc", clientType: "web" });
});
