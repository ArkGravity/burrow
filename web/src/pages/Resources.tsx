import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  App,
  Button,
  Drawer,
  Form,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import {
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  EditOutlined,
  DeleteOutlined,
  KeyOutlined,
  LinkOutlined,
  StopOutlined,
} from "@ant-design/icons";
import { api, write, APIError, type Row, type List } from "../lib/api";
import { can } from "../lib/access";
import { useSession } from "../lib/session";
import { useI18n, errorKey, type TranslationKey } from "../lib/i18n";
import { GroupTags, type GroupSummary } from "../components/GroupTags";

type Field = {
  key: string;
  label: TranslationKey;
  type?: "switch" | "password" | "urls" | "select" | "multi";
  source?: string;
  options?: { value: string; label: TranslationKey }[];
  required?: boolean;
  createOnly?: boolean;
};
const shared: Field[] = [
  { key: "name", label: "name", required: true },
  { key: "description", label: "description" },
];
const definitions: Record<
  string,
  { subtitle: TranslationKey; permission: string; fields: Field[] }
> = {
  users: {
    subtitle: "manageUsers",
    permission: "users:write",
    fields: [
      { key: "username", label: "username", required: true, createOnly: true },
      { key: "name", label: "name", required: true },
      { key: "email", label: "email" },
      { key: "enabled", label: "enabled", type: "switch" },
      { key: "localEnabled", label: "localEnabled", type: "switch" },
      {
        key: "password",
        label: "password",
        type: "password",
        createOnly: true,
      },
      { key: "roleIds", label: "roleIds", type: "multi", source: "roles" },
      { key: "groupIds", label: "groupIds", type: "multi", source: "groups" },
    ],
  },
  groups: {
    subtitle: "manageGroups",
    permission: "groups:write",
    fields: [
      ...shared,
      { key: "userIds", label: "userIds", type: "multi", source: "users" },
      { key: "roleIds", label: "roleIds", type: "multi", source: "roles" },
    ],
  },
  roles: {
    subtitle: "manageRoles",
    permission: "authorization:write",
    fields: [
      ...shared,
      {
        key: "permissionIds",
        label: "permissionIds",
        type: "multi",
        source: "permissions",
      },
    ],
  },
  permissions: {
    subtitle: "managePermissions",
    permission: "authorization:write",
    fields: [],
  },
  applications: {
    subtitle: "manageApplications",
    permission: "applications:write",
    fields: [
      { key: "name", label: "name", required: true },
      {
        key: "clientType",
        label: "clientType",
        type: "select",
        options: [
          { value: "web", label: "webClient" },
          { value: "spa", label: "spaClient" },
        ],
        required: true,
      },
      { key: "clientId", label: "clientId", createOnly: true },
      {
        key: "clientSecret",
        label: "clientSecret",
        type: "password",
        createOnly: true,
      },
      { key: "enabled", label: "enabled", type: "switch" },
      { key: "loginUrl", label: "loginUrl", required: true },
      { key: "icon", label: "icon" },
      {
        key: "redirectUris",
        label: "redirectUris",
        type: "urls",
        required: true,
      },
      {
        key: "postLogoutRedirectUris",
        label: "postLogoutRedirectUris",
        type: "urls",
      },
      { key: "origins", label: "origins", type: "urls" },
      { key: "allowWithoutPkce", label: "allowWithoutPkce", type: "switch" },
      { key: "localEnabled", label: "localEnabled", type: "switch" },
      {
        key: "providerIds",
        label: "providerIds",
        type: "multi",
        source: "providers",
      },
      { key: "roleIds", label: "roleIds", type: "multi", source: "roles" },
    ],
  },
  providers: {
    subtitle: "manageProviders",
    permission: "providers:write",
    fields: [
      { key: "name", label: "name", required: true },
      { key: "issuer", label: "issuer", required: true },
      { key: "clientId", label: "clientId", required: true },
      { key: "clientSecret", label: "clientSecret", type: "password" },
      { key: "enabled", label: "enabled", type: "switch" },
    ],
  },
};

export function Resources({ resource }: { resource: string }) {
  const { t } = useI18n();
  const { session } = useSession();
  const { message } = App.useApp();
  const def = definitions[resource];
  const [rows, setRows] = useState<Row[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<Row | null | undefined>();
  const [secret, setSecret] = useState("");
  const [saving, setSaving] = useState(false);
  const [options, setOptions] = useState<Record<string, Row[]>>({});
  const [passwordUser, setPasswordUser] = useState<Row>();
  const [identityUser, setIdentityUser] = useState<Row>();
  const [form] = Form.useForm();
  const clientType = Form.useWatch("clientType", form);
  const [passwordForm] = Form.useForm();
  const writable = can(session?.permissions || [], def.permission);
  const authWritable = can(session?.permissions || [], "authorization:write");
  const administrator = session?.administrator === true;
  const permissionLabel = (row: Row) => {
    const app = row.application as
      { name?: string; clientId?: string } | undefined;
    return app
      ? `${app.name || app.clientId} · ${t("applicationLoginPermission")} (${app.clientId})`
      : String(row.name || row.id);
  };
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await api<List>(
        `/${resource}?page=${page}&pageSize=20&search=${encodeURIComponent(search)}`,
      );
      setRows(result.items || []);
      setTotal(result.total);
    } catch (e) {
      setError((e as APIError).code);
    } finally {
      setLoading(false);
    }
  }, [resource, page, search]);
  useEffect(() => {
    void load();
  }, [load]);
  const open = async (row: Row | null) => {
    form.resetFields();
    setEditing(row);
    form.setFieldsValue(
      row
        ? Object.fromEntries(
            Object.entries(row).map(([k, v]) => [
              k,
              def.fields.find((f) => f.key === k)?.type === "urls" &&
              Array.isArray(v)
                ? v.join("\n")
                : v,
            ]),
          )
        : {
            enabled: true,
            localEnabled: true,
            clientType: "web",
            allowWithoutPkce: false,
          },
    );
    const sources = [
      ...new Set(def.fields.flatMap((f) => (f.source ? [f.source] : []))),
    ];
    const values: Record<string, Row[]> = {};
    for (const source of sources) {
      if (!can(session?.permissions || [], `${source}:read`)) continue;
      try {
        const first = await api<List>(`/${source}?pageSize=100`);
        let items = first.items || [];
        for (let p = 2; p <= Math.ceil(first.total / 100); p++) {
          const next = await api<List>(`/${source}?pageSize=100&page=${p}`);
          items = items.concat(next.items || []);
        }
        values[source] = items;
      } catch (e) {
        message.error(t(errorKey((e as APIError).code)));
      }
    }
    setOptions(values);
  };
  const submit = async (values: Record<string, unknown>) => {
    setSaving(true);
    try {
      for (const field of def.fields) {
        if (field.type === "urls")
          values[field.key] = String(values[field.key] || "")
            .split("\n")
            .map((x) => x.trim())
            .filter(Boolean);
        if (
          field.type === "multi" &&
          values[field.key] === undefined &&
          editing === null
        )
          values[field.key] = [];
      }
      if (!authWritable) {
        for (const key of ["roleIds", "groupIds", "userIds", "permissionIds"])
          delete values[key];
      }
      if (values.clientSecret === "") delete values.clientSecret;
      if (resource === "applications") {
        if (!values.clientId) delete values.clientId;
        if (editing || values.clientType === "spa") delete values.clientSecret;
        if (!administrator) delete values.allowWithoutPkce;
        else if (values.clientType === "spa") values.allowWithoutPkce = false;
      }
      const result = await write<Row>(
        `/${resource}${editing ? "/" + editing.id : ""}`,
        values,
        editing ? "PUT" : "POST",
      );
      if (result?.clientSecret) setSecret(String(result.clientSecret));
      setEditing(undefined);
      await load();
      message.success(t("success"));
    } catch (e) {
      message.error(t(errorKey((e as APIError).code)));
    } finally {
      setSaving(false);
    }
  };
  const action = async (path: string, method = "POST") => {
    try {
      const result = await api<Row>(path, {
        method,
        body: method === "POST" ? "{}" : undefined,
      });
      if (result?.clientSecret) setSecret(String(result.clientSecret));
      message.success(t("success"));
      await load();
    } catch (e) {
      message.error(t(errorKey((e as APIError).code)));
    }
  };
  const columns = [
    {
      title: t("name"),
      key: "name",
      render: (_: unknown, row: Row) => (
        <div className="record-name">
          <span className="record-avatar">
            {String(row.name || row.username || row.id)
              .slice(0, 1)
              .toUpperCase()}
          </span>
          <div>
            <strong>
              {resource === "permissions"
                ? permissionLabel(row)
                : String(row.name || row.username || row.id)}
            </strong>
            <div className="record-sub">
              {resource === "permissions"
                ? row.id
                : String(row.username || row.clientId || row.description || "")}
            </div>
          </div>
          {row.builtin === true && <Tag>{t("builtin")}</Tag>}
        </div>
      ),
    },
    ...(resource === "users"
      ? [
          { title: t("email"), dataIndex: "email", key: "email" },
          {
            title: t("groups"),
            key: "groups",
            render: (_: unknown, row: Row) => (
              <GroupTags
                groups={
                  Array.isArray(row.groups)
                    ? (row.groups as GroupSummary[])
                    : []
                }
              />
            ),
          },
        ]
      : []),
    ...(resource === "providers"
      ? [{ title: t("issuer"), dataIndex: "issuer", key: "issuer" }]
      : []),
    ...(["users", "applications", "providers"].includes(resource)
      ? [
          {
            title: t("status"),
            key: "status",
            render: (_: unknown, row: Row) => (
              <Tag color={row.enabled ? "success" : "default"}>
                {t(row.enabled ? "enabled" : "disabled")}
              </Tag>
            ),
          },
        ]
      : []),
    ...(resource !== "permissions" && writable
      ? [
          {
            title: t("actions"),
            key: "actions",
            width: 210,
            render: (_: unknown, row: Row) => (
              <Space wrap size={4}>
                <Button
                  size="small"
                  type="text"
                  aria-label={t("edit")}
                  disabled={row.builtin === true}
                  icon={<EditOutlined />}
                  onClick={() => void open(row)}
                />
                {resource === "users" && (
                  <>
                    <Button
                      size="small"
                      type="text"
                      aria-label={t("resetPassword")}
                      icon={<KeyOutlined />}
                      onClick={() => {
                        passwordForm.resetFields();
                        setPasswordUser(row);
                      }}
                    />
                    <Button
                      size="small"
                      type="text"
                      aria-label={t("identities")}
                      icon={<LinkOutlined />}
                      onClick={() => setIdentityUser(row)}
                    />
                    <Popconfirm
                      title={t("revokeSessions")}
                      onConfirm={() =>
                        action(`/users/${row.id}/revoke-sessions`)
                      }
                    >
                      <Button
                        size="small"
                        type="text"
                        aria-label={t("revokeSessions")}
                        icon={<StopOutlined />}
                      />
                    </Popconfirm>
                  </>
                )}
                {resource === "applications" && row.clientType !== "spa" && (
                  <Popconfirm
                    title={t("resetSecret")}
                    onConfirm={() => action(`/applications/${row.id}/secret`)}
                  >
                    <Button
                      size="small"
                      type="text"
                      aria-label={t("resetSecret")}
                      icon={<KeyOutlined />}
                    />
                  </Popconfirm>
                )}
                <Popconfirm
                  title={t("deleteTitle")}
                  description={t("deleteNote")}
                  onConfirm={() => action(`/${resource}/${row.id}`, "DELETE")}
                  okText={t("delete")}
                  cancelText={t("cancel")}
                >
                  <Button
                    size="small"
                    type="text"
                    danger
                    aria-label={t("delete")}
                    disabled={row.builtin === true}
                    icon={<DeleteOutlined />}
                  />
                </Popconfirm>
              </Space>
            ),
          },
        ]
      : []),
  ];
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">
            {t(
              ["applications", "providers"].includes(resource)
                ? "connections"
                : "identity",
            )}
          </div>
          <h1>{t(resource as TranslationKey)}</h1>
          <p>{t(def.subtitle)}</p>
        </div>
        {writable && resource !== "permissions" && (
          <Button
            aria-label={t("create")}
            type="primary"
            size="large"
            icon={<PlusOutlined />}
            onClick={() => void open(null)}
          >
            {t("create")}
          </Button>
        )}
      </div>
      {error && (
        <Alert
          showIcon
          type="error"
          title={t(errorKey(error))}
          className="form-alert"
        />
      )}
      <div className="data-panel">
        <div className="table-toolbar">
          <Input.Search
            aria-label={t("search")}
            placeholder={t("search")}
            prefix={<SearchOutlined />}
            allowClear
            onSearch={(value) => {
              setPage(1);
              setSearch(value);
            }}
            style={{ maxWidth: 340 }}
          />
          <Button
            aria-label={t("refresh")}
            icon={<ReloadOutlined />}
            onClick={() => void load()}
          />
        </div>
        <Table
          rowKey="id"
          loading={loading}
          dataSource={rows}
          columns={columns}
          scroll={{ x: 600 }}
          pagination={{
            current: page,
            pageSize: 20,
            total,
            onChange: setPage,
            showSizeChanger: false,
          }}
          locale={{ emptyText: t("empty") }}
        />
      </div>
      <Drawer
        title={`${t(editing ? "edit" : "create")} · ${t(resource as TranslationKey)}`}
        open={editing !== undefined}
        onClose={() => setEditing(undefined)}
        size={520}
        destroyOnHidden
        extra={
          <Button type="primary" loading={saving} onClick={() => form.submit()}>
            {t("save")}
          </Button>
        }
      >
        <Form
          form={form}
          layout="vertical"
          requiredMark={false}
          onFinish={submit}
        >
          {def.fields
            .filter(
              (f) =>
                (!f.createOnly || !editing) &&
                (resource !== "applications" ||
                  f.key !== "clientSecret" ||
                  clientType === "web") &&
                (f.key !== "allowWithoutPkce" || clientType === "web"),
            )
            .map((field) => {
              const restricted =
                ["roleIds", "groupIds", "userIds", "permissionIds"].includes(
                  field.key,
                ) && !authWritable;
              const optionDenied =
                field.source &&
                !can(session?.permissions || [], `${field.source}:read`);
              return (
                <Form.Item
                  key={field.key}
                  name={field.key}
                  label={t(field.label)}
                  valuePropName={field.type === "switch" ? "checked" : "value"}
                  rules={[
                    { required: field.required, message: t("required") },
                    ...(field.key === "email"
                      ? [{ type: "email" as const, message: t("invalidEmail") }]
                      : []),
                  ]}
                  extra={
                    resource === "applications" && field.key === "clientId"
                      ? t("applicationClientIdHint")
                      : resource === "applications" &&
                          field.key === "clientSecret"
                        ? t("applicationClientSecretHint")
                        : field.key === "allowWithoutPkce"
                          ? t("allowWithoutPkceHint")
                          : field.type === "urls"
                            ? t("linesHint")
                            : undefined
                  }
                >
                  {field.type === "switch" ? (
                    <Switch
                      disabled={
                        field.key === "allowWithoutPkce" && !administrator
                      }
                    />
                  ) : field.type === "password" ? (
                    <Input.Password autoComplete="new-password" />
                  ) : field.type === "urls" ? (
                    <Input.TextArea rows={3} />
                  ) : field.type === "multi" ? (
                    <Select
                      mode="multiple"
                      disabled={restricted || !!optionDenied}
                      optionFilterProp="label"
                      options={(options[field.source!] || [])
                        .filter(
                          (r) =>
                            !(
                              resource === "applications" &&
                              field.key === "roleIds" &&
                              r.builtin
                            ),
                        )
                        .map((r) => ({
                          value: r.id,
                          label:
                            field.source === "permissions"
                              ? permissionLabel(r)
                              : String(r.name || r.username || r.id),
                        }))}
                    />
                  ) : field.type === "select" ? (
                    <Select
                      disabled={field.key === "clientType" && !!editing}
                      options={field.options?.map((o) => ({
                        value: o.value,
                        label: t(o.label),
                      }))}
                    />
                  ) : (
                    <Input />
                  )}
                </Form.Item>
              );
            })}
        </Form>
      </Drawer>
      <Modal
        title={t("secretTitle")}
        open={!!secret}
        onCancel={() => setSecret("")}
        footer={<Button onClick={() => setSecret("")}>{t("close")}</Button>}
        destroyOnHidden
      >
        <Alert type="warning" title={t("secretNote")} className="form-alert" />
        <Typography.Paragraph copyable className="secret-value">
          {secret}
        </Typography.Paragraph>
      </Modal>
      <Modal
        title={t("resetPassword")}
        open={!!passwordUser}
        onCancel={() => setPasswordUser(undefined)}
        onOk={() => passwordForm.submit()}
        destroyOnHidden
      >
        <Form
          form={passwordForm}
          layout="vertical"
          onFinish={async (v) => {
            try {
              await write(`/users/${passwordUser!.id}/password`, v, "PUT");
              setPasswordUser(undefined);
              message.success(t("success"));
            } catch (e) {
              message.error(t(errorKey((e as APIError).code)));
            }
          }}
        >
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
        </Form>
      </Modal>
      {identityUser && (
        <IdentityDrawer
          user={identityUser}
          onClose={() => setIdentityUser(undefined)}
        />
      )}
    </>
  );
}

function IdentityDrawer({ user, onClose }: { user: Row; onClose: () => void }) {
  const { t } = useI18n();
  const { message } = App.useApp();
  const [rows, setRows] = useState<Row[]>([]);
  const [providers, setProviders] = useState<Row[]>([]);
  const [form] = Form.useForm();
  const load = useCallback(async () => {
    try {
      const result = await api<List | Row[]>(`/users/${user.id}/identities`);
      setRows(Array.isArray(result) ? result : result.items);
      const ps = await api<List>("/providers?pageSize=100");
      setProviders(ps.items);
    } catch (e) {
      message.error(t(errorKey((e as APIError).code)));
    }
  }, [user.id]);
  useEffect(() => {
    void load();
  }, [load]);
  return (
    <Drawer
      open
      title={`${t("identities")} · ${String(user.name || user.username)}`}
      onClose={onClose}
      size={520}
    >
      <Table
        rowKey="id"
        dataSource={rows}
        pagination={false}
        columns={[
          {
            title: t("providerId"),
            dataIndex: "providerId",
            render: (v) => String(providers.find((p) => p.id === v)?.name || v),
          },
          { title: t("subject"), dataIndex: "subject" },
          {
            title: t("actions"),
            render: (_, r) => (
              <Popconfirm
                title={t("unlink")}
                onConfirm={async () => {
                  try {
                    await api(`/users/${user.id}/identities/${r.id}`, {
                      method: "DELETE",
                    });
                    await load();
                  } catch (e) {
                    message.error(t(errorKey((e as APIError).code)));
                  }
                }}
              >
                <Button danger size="small">
                  {t("unlink")}
                </Button>
              </Popconfirm>
            ),
          },
        ]}
        locale={{ emptyText: t("noIdentities") }}
      />
      <Form
        form={form}
        layout="vertical"
        className="spaced-form"
        onFinish={async (v) => {
          try {
            await write(`/users/${user.id}/identities`, v);
            form.resetFields();
            await load();
          } catch (e) {
            message.error(t(errorKey((e as APIError).code)));
          }
        }}
      >
        <Form.Item
          name="providerId"
          label={t("providerId")}
          rules={[{ required: true, message: t("required") }]}
        >
          <Select
            options={providers.map((p) => ({
              value: p.id,
              label: String(p.name),
            }))}
          />
        </Form.Item>
        <Form.Item
          name="subject"
          label={t("subject")}
          rules={[{ required: true, message: t("required") }]}
        >
          <Input />
        </Form.Item>
        <Button type="primary" htmlType="submit">
          {t("link")}
        </Button>
      </Form>
    </Drawer>
  );
}
