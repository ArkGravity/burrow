import { describe, expect, it } from "vitest";
import { can, safeRedirect } from "./access";
describe("UI authorization and navigation", () => {
  it("defaults to deny and only grants known permissions", () => {
    expect(can([], "users:write")).toBe(false);
    expect(can(["users:read"], "users:write")).toBe(false);
    expect(can(["users:read"], "users:read")).toBe(true);
  });
  it("never navigates to external or protocol relative authorization redirects", () => {
    expect(safeRedirect("https://evil.test")).toBe("/");
    expect(safeRedirect("//evil.test")).toBe("/");
    expect(safeRedirect("/\\evil.test")).toBe("/");
    expect(safeRedirect("/oidc/login?requestId=abc")).toBe(
      "/oidc/login?requestId=abc",
    );
  });
});
