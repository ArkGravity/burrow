import { useEffect, useState } from "react";
import { Alert, App, Button, Card, Empty, Form, Input, Skeleton } from "antd";
import {
  ArrowRightOutlined,
  AppstoreOutlined,
  TeamOutlined,
  CheckCircleOutlined,
  WarningOutlined,
} from "@ant-design/icons";
import { api, write, APIError, type Row, type List } from "../lib/api";
import { can } from "../lib/access";
import { useSession } from "../lib/session";
import { useI18n, errorKey } from "../lib/i18n";
import { Preferences } from "../components/Preferences";
import { PasswordForm } from "./Auth";
export function HomePage() {
  const { t } = useI18n();
  const { session } = useSession();
  const [apps, setApps] = useState<Row[]>([]);
  const [stats, setStats] = useState<Record<string, number>>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    Promise.all([
      api<List | Row[]>("/me/apps"),
      can(session?.permissions || [], "dashboard:read")
        ? api<Record<string, number>>("/dashboard")
        : Promise.resolve(undefined),
    ])
      .then(([a, s]) => {
        if (live) {
          setApps(Array.isArray(a) ? a : a.items);
          setStats(s);
        }
      })
      .catch((e) => {
        if (live) setError(e.code);
      })
      .finally(() => {
        if (live) setLoading(false);
      });
    return () => {
      live = false;
    };
  }, [session]);
  return (
    <>
      <div className="page-heading home-heading">
        <div>
          <div className="eyebrow">
            {t("welcome")}, {session?.user.name || session?.user.username}
          </div>
          <h1>{t("portal")}</h1>
          <p>{t("portalSub")}</p>
        </div>
        <div className="home-stamp">
          <span className="status-dot" /> OIDC · SSO
        </div>
      </div>
      {error && <Alert type="error" title={t(errorKey(error))} showIcon />}
      {loading ? (
        <Skeleton active />
      ) : (
        <>
          {stats && (
            <div className="stats-grid">
              {(
                [
                  { key: "users", label: "totalUsers", icon: <TeamOutlined /> },
                  {
                    key: "applications",
                    label: "totalApps",
                    icon: <AppstoreOutlined />,
                  },
                  {
                    key: "loginSuccess",
                    label: "loginSuccess",
                    icon: <CheckCircleOutlined />,
                  },
                  {
                    key: "loginFailure",
                    label: "loginFailure",
                    icon: <WarningOutlined />,
                  },
                ] as const
              ).map((x) => (
                <div className="stat-card" key={x.key}>
                  <div className="stat-top">
                    <span>{t(x.label)}</span>
                    {x.icon}
                  </div>
                  <strong>{stats[x.key] ?? 0}</strong>
                  <div className="stat-rule" />
                </div>
              ))}
            </div>
          )}
          <div className="section-heading">
            <h2>{t("yourApps")}</h2>
            <span className="count-badge">{apps.length}</span>
          </div>
          {apps.length === 0 ? (
            <div className="empty-panel">
              <Empty
                description={
                  <>
                    <h3>{t("noApps")}</h3>
                    <p>{t("noAppsSub")}</p>
                  </>
                }
              />
            </div>
          ) : (
            <div className="apps-grid">
              {apps.map((app) => (
                <a
                  className="app-card"
                  key={app.id}
                  href={String(app.loginUrl)}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <div className="app-icon">
                    {app.icon ? (
                      <img
                        src={String(app.icon)}
                        alt=""
                        referrerPolicy="no-referrer"
                      />
                    ) : (
                      <AppstoreOutlined />
                    )}
                  </div>
                  <h3>{String(app.name)}</h3>
                  <span>{t("openApp")}</span>
                  <ArrowRightOutlined className="app-arrow" />
                </a>
              ))}
            </div>
          )}
        </>
      )}
    </>
  );
}
export function ProfilePage() {
  const { t } = useI18n();
  const { session, reload } = useSession();
  const { message } = App.useApp();
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">{t("workspace")}</div>
          <h1>{t("profile")}</h1>
        </div>
      </div>
      <div className="profile-grid">
        <Card title={t("detail")}>
          <Form
            layout="vertical"
            initialValues={session?.user}
            onFinish={async (v) => {
              try {
                await write("/me", v, "PUT");
                await reload();
                message.success(t("success"));
              } catch (e) {
                message.error(t(errorKey((e as APIError).code)));
              }
            }}
          >
            <Form.Item label={t("username")}>
              <Input disabled value={session?.user.username} />
            </Form.Item>
            <Form.Item
              name="name"
              label={t("name")}
              rules={[{ required: true, message: t("required") }]}
            >
              <Input />
            </Form.Item>
            <Form.Item label={t("email")}>
              <Input disabled value={session?.user.email} />
            </Form.Item>
            <Button type="primary" htmlType="submit">
              {t("save")}
            </Button>
          </Form>
          <div className="spaced-form">
            <h3>{t("settings")}</h3>
            <Preferences />
          </div>
        </Card>
        <Card title={t("security")}>
          <Alert
            type={session?.mfaRequired ? "success" : "info"}
            showIcon
            title={t(session?.mfaRequired ? "mfaBound" : "mfaDisabled")}
            className="form-alert"
          />
          {session?.mfaRequired && (
            <p className="muted">{t("mfaRecoveryHint")}</p>
          )}
          <PasswordForm />
        </Card>
      </div>
    </>
  );
}
