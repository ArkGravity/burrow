export function applicationPayload(
  values: Record<string, unknown>,
): Record<string, unknown> {
  const protocol = values.protocol || "oidc";
  if (protocol !== "oidc" && protocol !== "saml")
    throw new Error("invalid protocol");
  const fields =
    protocol === "saml"
      ? ["name", "enabled", "icon", "loginUrl", "roleIds"]
      : [
          "name",
          "enabled",
          "icon",
          "loginUrl",
          "roleIds",
          "clientId",
          "clientType",
          "clientSecret",
          "redirectUris",
          "postLogoutRedirectUris",
          "origins",
          "allowWithoutPkce",
        ];
  const result: Record<string, unknown> = {};
  for (const field of fields)
    if (values[field] !== undefined) result[field] = values[field];
  result.protocol = protocol;
  if (protocol === "saml") {
    const saml = values.saml as { metadataXml?: string } | undefined;
    if (saml?.metadataXml !== undefined)
      result.saml = { metadataXml: saml.metadataXml };
  }
  return result;
}
