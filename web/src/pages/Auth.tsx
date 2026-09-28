import { useEffect, useState } from "react";
import { Alert, App, Button, Divider, Form, Input, Space } from "antd";
import {
  ArrowRightOutlined,
  SafetyCertificateOutlined,
} from "@ant-design/icons";
import { Navigate, useSearchParams } from "react-router-dom";
import { api, write, APIError, resetCSRF, type Session } from "../lib/api";
import { useI18n, errorKey } from "../lib/i18n";
import { useSession } from "../lib/session";
import { safeRedirect } from "../lib/access";
import { Preferences } from "../components/Preferences";

export function AuthFrame({ children }: { children: React.ReactNode }) {
  const { t } = useI18n();
  return (
    <div className="auth-layout">
      <aside className="auth-story">
        <a href="/" className="brand light">
          <span className="brand-mark">b.</span>burrow
          <span className="brand-dot">/</span>
        </a>
        <div className="auth-story-body">
          <div className="eyebrow">
            <span className="status-dot" />
            {t("secure")}
          </div>
          <h1>{t("loginTitle")}</h1>
          <p>{t("loginSub")}</p>
          <div className="orbit-art" aria-hidden="true">
            <div />
            <div />
            <span>b.</span>
          </div>
        </div>
        <span className="auth-footer">{t("footer")}</span>
      </aside>
      <section className="auth-main">
        <div className="auth-preferences">
          <Preferences />
        </div>
        <div className="auth-card">{children}</div>
        <span className="auth-bottom">
          <SafetyCertificateOutlined /> Burrow Identity
        </span>
      </section>
    </div>
  );
}
export function LoginPage() {
  const { t } = useI18n();
  const { session } = useSession();
  const [query] = useSearchParams();
  const requestId = query.get("requestId") || "";
  const [context, setContext] = useState<{
    localEnabled: boolean;
    providers: { id: string; name: string }[];
    applicationName?: string;
  }>({ localEnabled: true, providers: [] });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let live = true;
    (requestId
      ? api<typeof context>(
          `/auth/context?requestId=${encodeURIComponent(requestId)}`,
        )
      : api<{ id: string; name: string }[]>("/auth/providers").then(
          (providers) => ({ localEnabled: true, providers }),
        )
    )
      .then((v) => {
        if (live) setContext(v);
      })
      .catch((e) => {
        if (live) setError(e.code || "REQUEST_FAILED");
      });
    return () => {
      live = false;
    };
  }, [requestId]);
  if (session && !requestId)
    return (
      <Navigate
        to={session.user.mustChangePassword ? "/change-password" : "/"}
        replace
      />
    );
  const submit = async (values: { username: string; password: string }) => {
    setBusy(true);
    setError("");
    try {
      const value = await write<Session>("/auth/login", {
        ...values,
        requestId,
      });
      resetCSRF();
      window.location.assign(
        value.user.mustChangePassword
          ? `/change-password${requestId ? "?requestId=" + encodeURIComponent(requestId) : ""}`
          : safeRedirect(value.redirect),
      );
    } catch (e) {
      setError((e as APIError).code);
    } finally {
      setBusy(false);
    }
  };
  return (
    <AuthFrame>
      <div className="eyebrow">
        {context.applicationName || "BURROW IDENTITY"}
      </div>
      <h2>{t("welcome")}</h2>
      <p className="muted">{t("loginSub")}</p>
      {(error || query.get("error")) && (
        <Alert
          type="error"
          showIcon
          title={t(errorKey(error || query.get("error")!))}
          className="form-alert"
        />
      )}
      {context.localEnabled && (
        <Form layout="vertical" onFinish={submit} requiredMark={false}>
          <Form.Item
            name="username"
            label={t("username")}
            rules={[{ required: true, message: t("required") }]}
          >
            <Input size="large" autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label={t("password")}
            rules={[{ required: true, message: t("required") }]}
          >
            <Input.Password size="large" autoComplete="current-password" />
          </Form.Item>
          <Button
            aria-label={t("signIn")}
            size="large"
            type="primary"
            htmlType="submit"
            block
            loading={busy}
          >
            {t("signIn")}
            <ArrowRightOutlined />
          </Button>
        </Form>
      )}
      {context.providers.length > 0 && (
        <>
          <Divider plain>{t("externalLogin")}</Divider>
          <Space orientation="vertical" className="full-width">
            {context.providers.map((p) => (
              <Button
                key={p.id}
                size="large"
                block
                href={`/api/v1/auth/providers/${encodeURIComponent(p.id)}/login${requestId ? "?requestId=" + encodeURIComponent(requestId) : ""}`}
              >
                {p.name}
                <ArrowRightOutlined />
              </Button>
            ))}
          </Space>
        </>
      )}
    </AuthFrame>
  );
}
export function PasswordForm({ required = false }: { required?: boolean }) {
  const { t } = useI18n();
  const { message } = App.useApp();
  const [query] = useSearchParams();
  const [busy, setBusy] = useState(false);
  const [form] = Form.useForm();
  const submit = async (values: Record<string, string>) => {
    setBusy(true);
    try {
      const result = await write<{ redirect?: string }>("/me/password", {
        ...values,
        requestId: query.get("requestId") || "",
      });
      resetCSRF();
      if (required) window.location.assign(safeRedirect(result?.redirect));
      else {
        message.success(t("success"));
        form.resetFields();
      }
    } catch (e) {
      message.error(t(errorKey((e as APIError).code)));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
      <Form.Item
        name="currentPassword"
        label={t("currentPassword")}
        rules={[{ required: true, message: t("required") }]}
      >
        <Input.Password autoComplete="current-password" />
      </Form.Item>
      <Form.Item
        name="password"
        label={t("newPassword")}
        rules={[
          { required: true, message: t("required") },
          { min: 12, message: t("passwordHint") },
        ]}
      >
        <Input.Password autoComplete="new-password" />
      </Form.Item>
      <Form.Item
        name="confirm"
        label={t("confirmPassword")}
        dependencies={["password"]}
        rules={[
          { required: true, message: t("required") },
          ({ getFieldValue }) => ({
            validator(_, v) {
              return !v || getFieldValue("password") === v
                ? Promise.resolve()
                : Promise.reject(new Error(t("passwordMismatch")));
            },
          }),
        ]}
      >
        <Input.Password autoComplete="new-password" />
      </Form.Item>
      <Button htmlType="submit" type="primary" loading={busy}>
        {t("changePassword")}
      </Button>
    </Form>
  );
}
export function ChangePasswordPage() {
  const { t } = useI18n();
  return (
    <AuthFrame>
      <h2>{t("changePassword")}</h2>
      <Alert
        type="info"
        showIcon
        title={t("mustChange")}
        className="form-alert"
      />
      <PasswordForm required />
    </AuthFrame>
  );
}
export function LogoutPage() {
  const { t } = useI18n();
  const { message } = App.useApp();
  const [query] = useSearchParams();
  return (
    <AuthFrame>
      <h2>{t("logoutTitle")}</h2>
      <p>{t("logoutNote")}</p>
      <Button
        type="primary"
        onClick={async () => {
          try {
            let target = "/login";
            if (
              query.has("id_token_hint") ||
              query.has("post_logout_redirect_uri")
            ) {
              const result = await write<{ redirect: string }>("/oidc/logout", {
                idTokenHint: query.get("id_token_hint") || "",
                postLogoutRedirectUri:
                  query.get("post_logout_redirect_uri") || "",
                state: query.get("state") || "",
              });
              // The server validates the exact registered logout URI and the ID-token hint.
              const url = new URL(result.redirect, window.location.origin);
              if (
                !["http:", "https:"].includes(url.protocol) ||
                url.username ||
                url.password
              )
                throw new APIError("INVALID_LOGOUT", 400);
              target = url.href;
            } else {
              await write("/auth/logout", {});
            }
            resetCSRF();
            window.location.assign(target);
          } catch (e) {
            message.error(t(errorKey((e as APIError).code)));
          }
        }}
      >
        {t("signOut")}
      </Button>
    </AuthFrame>
  );
}
