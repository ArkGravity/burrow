export type LoginRequest = { requestType: "oidc" | "saml"; requestId: string };
export function readLoginRequest(search: string): LoginRequest {
  const query = new URLSearchParams(search);
  if (
    query.getAll("requestType").length > 1 ||
    query.getAll("requestId").length > 1
  )
    throw new Error("invalid_request_type");
  const requestType = query.get("requestType") || "oidc";
  const requestId = query.get("requestId") || "";
  if (requestType !== "oidc" && requestType !== "saml")
    throw new Error("invalid_request_type");
  if (requestType === "saml" && !requestId)
    throw new Error("invalid_transaction");
  return { requestType, requestId };
}
export function sameLoginRequest(
  a: { requestType?: string; requestId: string },
  b: { requestType?: string; requestId: string },
) {
  return (
    (a.requestType || "oidc") === (b.requestType || "oidc") &&
    a.requestId === b.requestId
  );
}
