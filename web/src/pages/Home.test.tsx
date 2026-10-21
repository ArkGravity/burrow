import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api, type Row } from "../lib/api";
import { I18nContext, type Language } from "../lib/i18n";
import { HomePage } from "./Home";

vi.mock("../lib/api", async (original) => ({
  ...(await original<typeof import("../lib/api")>()),
  api: vi.fn(),
}));
afterEach(cleanup);

function portal(apps: Row[], language: Language = "en") {
  vi.mocked(api).mockResolvedValue({ items: apps });
  render(
    <I18nContext.Provider value={{ language, setLanguage: vi.fn() }}>
      <HomePage />
    </I18nContext.Provider>,
  );
}

it("separates assigned OIDC and SAML applications while preserving launch links", async () => {
  portal([
    {
      id: "grafana",
      name: "Grafana",
      protocol: "oidc",
      loginUrl: "https://grafana.example.test/login",
    },
    {
      id: "jenkins",
      name: "Jenkins",
      protocol: "saml",
      loginUrl: "https://jenkins.example.test/securityRealm/commenceLogin",
    },
    {
      id: "legacy",
      name: "Legacy OIDC",
      loginUrl: "https://legacy.example.test/login",
    },
  ]);
  const oidc = await screen.findByRole("region", { name: "OIDC" });
  const saml = screen.getByRole("region", { name: "SAML" });
  expect(within(oidc).getAllByRole("link")).toHaveLength(2);
  expect(within(saml).getAllByRole("link")).toHaveLength(1);
  expect(within(oidc).queryByRole("link", { name: /Jenkins/ })).toBeNull();
  expect(within(saml).getByRole("link", { name: /Jenkins/ })).toHaveAttribute(
    "href",
    "https://jenkins.example.test/securityRealm/commenceLogin",
  );
});

it("omits protocol groups without assigned applications", async () => {
  portal([
    {
      id: "jenkins",
      name: "Jenkins",
      protocol: "saml",
      loginUrl: "https://jenkins.example.test/",
    },
  ]);
  expect(
    await screen.findByRole("region", { name: "SAML" }),
  ).toBeInTheDocument();
  expect(screen.queryByRole("region", { name: "OIDC" })).toBeNull();
});

it("keeps the translated unassigned state", async () => {
  portal([], "zh-CN");
  expect(await screen.findByText("暂无可访问的应用")).toBeInTheDocument();
  expect(screen.queryByRole("region", { name: "OIDC" })).toBeNull();
  expect(screen.queryByRole("region", { name: "SAML" })).toBeNull();
});
