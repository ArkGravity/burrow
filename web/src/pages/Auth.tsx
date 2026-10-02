import { useEffect, useState } from "react";
import {
  Alert,
  App,
  Button,
  Form,
  Input,
  Space,
  QRCode,
  Typography,
  Spin,
} from "antd";
import { ArrowRightOutlined } from "@ant-design/icons";
import { Navigate, useSearchParams } from "react-router-dom";
import { api, write, APIError, resetCSRF, type LoginState } from "../lib/api";
import { useI18n, errorKey } from "../lib/i18n";
import { useSession } from "../lib/session";
import { safeRedirect } from "../lib/access";
import { Preferences } from "../components/Preferences";
import { BrandMark } from "../components/BrandMark";

export function AuthFrame({ children }: { children: React.ReactNode }) {
  const { t } = useI18n();
  return (
    <div className="auth-layout">
      <aside className="auth-story">
        <a href="/" className="brand light">
          <BrandMark />
          burrow
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
            <BrandMark />
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
          <BrandMark /> Burrow Identity
        </span>
      </section>
    </div>
  );
}
export function continueLogin(value: LoginState) {
  resetCSRF();
  window.location.assign(
    value.step === "complete"
      ? safeRedirect(value.redirect)
      : value.step === "password"
        ? "/change-password"
        : "/mfa",
  );
}
export function LoginPage() {
  const { t } = useI18n();
  const { session } = useSession();
  const [query] = useSearchParams();
  const requestId = query.get("requestId") || "";
  const [context, setContext] = useState<{ applicationName?: string }>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    void api<LoginState>("/auth/login/status")
      .then((value) => {
        if (value.requestId === requestId) continueLogin(value);
      })
      .catch(() => {});
  }, [requestId]);
  useEffect(() => {
    let live = true;
    (requestId
      ? api<typeof context>(
          `/auth/context?requestId=${encodeURIComponent(requestId)}`,
        )
      : Promise.resolve({})
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
      const value = await write<LoginState>("/auth/login", {
        ...values,
        requestId,
      });
      continueLogin(value);
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
      const result = await write<LoginState>(
        required ? "/auth/login/password" : "/me/password",
        {
          ...values,
          requestId: query.get("requestId") || "",
        },
      );
      resetCSRF();
      if (required) continueLogin(result);
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
      {!required && (
        <Form.Item
          name="currentPassword"
          label={t("currentPassword")}
          rules={[{ required: true, message: t("required") }]}
        >
          <Input.Password autoComplete="current-password" />
        </Form.Item>
      )}
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
  return <MFAPage />;
}
export function MFAPage() {
  const { t } = useI18n();
  const [state, setState] = useState<LoginState>();
  const [binding, setBinding] = useState<{ secret: string; uri: string }>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [expired, setExpired] = useState(false);
  useEffect(() => {
    let live = true;
    void api<LoginState>("/auth/login/status")
      .then(async (value) => {
        if (!live) return;
        setState(value);
        if (value.step === "bind") {
          const secret = await write<{ secret: string; uri: string }>(
            "/auth/login/bind",
            {},
          );
          if (live) setBinding(secret);
        }
      })
      .catch((e) => {
        if (live) setError(e.code || "REQUEST_FAILED");
      });
    return () => {
      live = false;
    };
  }, []);
  useEffect(() => {
    if (!state) return;
    const timer = window.setInterval(() => {
      if (Date.now() >= Date.parse(state.expiresAt)) {
        setExpired(true);
        setBinding(undefined);
        setError("INVALID_TRANSACTION");
      }
    }, 1000);
    return () => window.clearInterval(timer);
  }, [state]);
  const submit = async (values: { code: string }) => {
    setBusy(true);
    setError("");
    try {
      continueLogin(await write<LoginState>("/auth/login/verify", values));
    } catch (e) {
      setError((e as APIError).code);
    } finally {
      setBusy(false);
    }
  };
  const restart = async () => {
    try {
      await write("/auth/login/cancel", {});
      window.location.assign(
        "/login" +
          (state?.requestId
            ? "?requestId=" + encodeURIComponent(state.requestId)
            : ""),
      );
    } catch (e) {
      setError((e as APIError).code);
    }
  };
  return (
    <AuthFrame>
      <h2>
        {t(
          state?.step === "password"
            ? "changePassword"
            : state?.step === "bind"
              ? "mfaBind"
              : "mfaVerify",
        )}
      </h2>
      {error && (
        <Alert
          type="error"
          showIcon
          title={t(errorKey(error))}
          className="form-alert"
        />
      )}
      {!state && !error && <Spin />}
      {state &&
        !expired &&
        (state.step === "password" ? (
          <>
            <Alert
              type="info"
              showIcon
              title={t("mustChange")}
              className="form-alert"
            />
            <PasswordForm required />
          </>
        ) : (
          <>
            <p className="muted">
              {t(state.step === "bind" ? "mfaBindHint" : "mfaVerifyHint")}
            </p>
            {state.step === "bind" && binding && (
              <Space orientation="vertical" className="form-alert">
                <QRCode value={binding.uri} bgColor="#fff" color="#000" />
                <Typography.Text>{t("mfaSecret")}</Typography.Text>
                <Typography.Text code copyable data-testid="mfa-secret">
                  {binding.secret}
                </Typography.Text>
              </Space>
            )}
            <Form layout="vertical" onFinish={submit} requiredMark={false}>
              <Form.Item
                name="code"
                label={t("mfaCode")}
                rules={[
                  { required: true, message: t("required") },
                  { pattern: /^[0-9]{6}$/, message: t("mfaCodeHint") },
                ]}
              >
                <Input
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  maxLength={6}
                  size="large"
                />
              </Form.Item>
              <Button
                block
                type="primary"
                htmlType="submit"
                loading={busy}
                disabled={state.step === "bind" && !binding}
              >
                {t("mfaContinue")}
              </Button>
            </Form>
          </>
        ))}
      <p className="muted">{t("mfaRecoveryHint")}</p>
      <Button type="link" onClick={() => void restart()}>
        {t("mfaRestart")}
      </Button>
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
