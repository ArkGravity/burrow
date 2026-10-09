import React, { useCallback, useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import {
  BrowserRouter,
  Navigate,
  NavLink,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import {
  App as AntApp,
  Alert,
  Avatar,
  Button,
  ConfigProvider,
  Result,
  Spin,
  theme,
} from "antd";
import enUS from "antd/locale/en_US";
import zhCN from "antd/locale/zh_CN";
import {
  AppstoreOutlined,
  HomeOutlined,
  TeamOutlined,
  UserOutlined,
  SafetyOutlined,
  KeyOutlined,
  LogoutOutlined,
} from "@ant-design/icons";
import { api, write, APIError, resetCSRF, type Session } from "./lib/api";
import { can } from "./lib/access";
import {
  I18nContext,
  useI18n,
  errorKey,
  type Language,
  type TranslationKey,
} from "./lib/i18n";
import { SessionContext, useSession } from "./lib/session";
import { Preferences, ThemeContext, type Mode } from "./components/Preferences";
import { BrandMark } from "./components/BrandMark";
import {
  ChangePasswordPage,
  LoginPage,
  LogoutPage,
  MFAPage,
} from "./pages/Auth";
import { HomePage, ProfilePage } from "./pages/Home";
import { Resources } from "./pages/Resources";
import "./style.css";

const navigation = [
  { key: "home", path: "/", icon: <HomeOutlined />, group: "workspace" },
  { key: "users", path: "/users", icon: <UserOutlined />, group: "identity" },
  { key: "groups", path: "/groups", icon: <TeamOutlined />, group: "identity" },
  { key: "roles", path: "/roles", icon: <SafetyOutlined />, group: "identity" },
  {
    key: "permissions",
    path: "/permissions",
    icon: <KeyOutlined />,
    group: "identity",
  },
  {
    key: "applications",
    path: "/applications",
    icon: <AppstoreOutlined />,
    group: "connections",
  },
] as const;
function Shell() {
  const { session } = useSession();
  const { t } = useI18n();
  const location = useLocation();
  const { message } = AntApp.useApp();
  const [signingOut, setSigningOut] = useState(false);
  const signOut = async () => {
    setSigningOut(true);
    try {
      await write("/auth/logout", {});
      resetCSRF();
      window.location.assign("/login");
    } catch (e) {
      message.error(t(errorKey((e as APIError).code)));
      setSigningOut(false);
    }
  };
  if (!session) return <Navigate to="/login" replace />;
  if (session.user.mustChangePassword)
    return <Navigate to="/change-password" replace />;
  const active = navigation.find((x) => x.path === location.pathname);
  return (
    <div className="workspace">
      <aside className="sidebar">
        <NavLink to="/" className="workspace-label" aria-label="Burrow">
          <BrandMark />
          <div>
            <strong>Burrow</strong>
            <small>{t("admin")}</small>
          </div>
          <span className="status-dot" />
        </NavLink>
        <nav>
          {(["workspace", "identity", "connections"] as const).map((group) => {
            const items = navigation.filter(
              (n) =>
                n.group === group &&
                (n.key === "home" || can(session.permissions, `${n.key}:read`)),
            );
            return (
              items.length > 0 && (
                <div className="nav-group" key={group}>
                  <div className="nav-label">{t(group)}</div>
                  {items.map((item) => (
                    <NavLink
                      aria-label={t(item.key)}
                      key={item.key}
                      end
                      to={item.path}
                      className={({ isActive }) =>
                        "nav-item" + (isActive ? " active" : "")
                      }
                    >
                      {item.icon}
                      <span>{t(item.key)}</span>
                      {item.key === "home" && (
                        <span className="nav-indicator" />
                      )}
                    </NavLink>
                  ))}
                </div>
              )
            );
          })}
        </nav>
        <div className="sidebar-bottom">
          <NavLink to="/profile" className="user-block">
            <Avatar style={{ background: "#dbe8db", color: "#2a4c42" }}>
              {(session.user.name || session.user.username)
                .slice(0, 1)
                .toUpperCase()}
            </Avatar>
            <div>
              <strong>{session.user.name || session.user.username}</strong>
              <small>{t("profile")}</small>
            </div>
          </NavLink>
          <Button
            type="text"
            aria-label={t("signOut")}
            icon={<LogoutOutlined />}
            loading={signingOut}
            onClick={() => void signOut()}
          />
        </div>
      </aside>
      <div className="main-workspace">
        <header className="topbar">
          <span className="breadcrumb">
            Burrow <span>/</span> <strong>{t(active?.key || "profile")}</strong>
          </span>
          <Preferences />
        </header>
        <main>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/profile" element={<ProfilePage />} />
            {navigation
              .filter((n) => n.key !== "home")
              .map((n) => (
                <Route
                  key={n.key}
                  path={n.path}
                  element={
                    can(session.permissions, `${n.key}:read`) ? (
                      <Resources key={n.key} resource={n.key} />
                    ) : (
                      <Result status="403" subTitle={t("denied")} />
                    )
                  }
                />
              ))}
            <Route
              path="*"
              element={
                <Result
                  status="404"
                  extra={<Button href="/">{t("back")}</Button>}
                />
              }
            />
          </Routes>
        </main>
        <footer className="workspace-footer">
          {t("footer")}
          <span>v0.1</span>
        </footer>
      </div>
    </div>
  );
}
function Root() {
  const [session, setSession] = useState<Session | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [language, setLang] = useState<Language>(() =>
    localStorage.getItem("burrow.language") === "zh-CN" ? "zh-CN" : "en",
  );
  const [mode, setTheme] = useState<Mode>(() => {
    const v = localStorage.getItem("burrow.theme");
    return v === "dark" || v === "light" ? v : "system";
  });
  const [systemDark, setSystemDark] = useState(
    () => window.matchMedia("(prefers-color-scheme: dark)").matches,
  );
  const reload = useCallback(async () => {
    try {
      const value = await api<Session>("/me");
      setSession(value);
      if (value.user.language === "en" || value.user.language === "zh-CN")
        setLang(value.user.language);
      if (["light", "dark", "system"].includes(value.user.theme))
        setTheme(value.user.theme as Mode);
      setError(false);
    } catch (e) {
      if ((e as { status: number }).status === 401) setSession(null);
      else setError(true);
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void reload();
  }, [reload]);
  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const change = () => setSystemDark(media.matches);
    media.addEventListener("change", change);
    return () => media.removeEventListener("change", change);
  }, []);
  const dark = mode === "dark" || (mode === "system" && systemDark);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    document.documentElement.lang = language;
    localStorage.setItem("burrow.language", language);
    localStorage.setItem("burrow.theme", mode);
  }, [dark, language, mode]);
  const setLanguage = (v: Language) => {
    setLang(v);
    if (session && !session.user.mustChangePassword)
      void write("/me", { language: v }, "PUT").catch(() => setError(true));
  };
  const setMode = (v: Mode) => {
    setTheme(v);
    if (session && !session.user.mustChangePassword)
      void write("/me", { theme: v }, "PUT").catch(() => setError(true));
  };
  return (
    <I18nContext.Provider value={{ language, setLanguage }}>
      <ThemeContext.Provider value={{ mode, setMode }}>
        <SessionContext.Provider value={{ session, reload }}>
          <ConfigProvider
            locale={language === "zh-CN" ? zhCN : enUS}
            theme={{
              algorithm: dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
              token: {
                colorPrimary: "#35785d",
                borderRadius: 8,
                fontFamily:
                  'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
                controlHeight: 38,
                colorBgLayout: dark ? "#101917" : "#f7f8f5",
              },
              components: {
                Table: { headerBg: dark ? "#1d2925" : "#fafbf8" },
                Button: { primaryShadow: "none" },
              },
            }}
          >
            <AntApp>
              {loading ? (
                <div className="loading-screen">
                  <Spin size="large" />
                </div>
              ) : (
                <>
                  {error && (
                    <Alert
                      type="error"
                      closable
                      title={
                        language === "zh-CN"
                          ? "服务暂时不可用，请刷新后重试。"
                          : "The service is unavailable. Please refresh to retry."
                      }
                    />
                  )}
                  <BrowserRouter>
                    <Routes>
                      <Route path="/login" element={<LoginPage />} />
                      <Route path="/mfa" element={<MFAPage />} />
                      <Route
                        path="/change-password"
                        element={<ChangePasswordPage />}
                      />
                      <Route
                        path="/logout"
                        element={
                          session ? (
                            <LogoutPage />
                          ) : (
                            <Navigate to="/login" replace />
                          )
                        }
                      />
                      <Route path="*" element={<Shell />} />
                    </Routes>
                  </BrowserRouter>
                </>
              )}
            </AntApp>
          </ConfigProvider>
        </SessionContext.Provider>
      </ThemeContext.Provider>
    </I18nContext.Provider>
  );
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Root />
  </React.StrictMode>,
);
