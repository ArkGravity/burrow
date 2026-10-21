import { describe, expect, it } from "vitest";
import { readLoginRequest, sameLoginRequest } from "./login-request";

describe("login request", () => {
  it("defaults existing requests to OIDC", () => {
    expect(readLoginRequest("?requestId=old")).toEqual({
      requestType: "oidc",
      requestId: "old",
    });
  });
  it("preserves SAML and rejects unknown/ambiguous requests", () => {
    expect(readLoginRequest("?requestType=saml&requestId=new")).toEqual({
      requestType: "saml",
      requestId: "new",
    });
    for (const query of [
      "?requestType=https://bad.test&requestId=x",
      "?requestType=saml",
      "?requestType=saml&requestType=oidc&requestId=x",
      "?requestId=a&requestId=b",
    ])
      expect(() => readLoginRequest(query)).toThrow();
  });
  it("compares protocol and ID when resuming saved login state", () => {
    expect(
      sameLoginRequest(
        { requestType: "saml", requestId: "same" },
        { requestType: "oidc", requestId: "same" },
      ),
    ).toBe(false);
    expect(
      sameLoginRequest(
        { requestId: "old" },
        { requestType: "oidc", requestId: "old" },
      ),
    ).toBe(true);
  });
});
