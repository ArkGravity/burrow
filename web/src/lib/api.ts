export interface User {
  id: string;
  username: string;
  name: string;
  email: string;
  language: string;
  theme: string;
  mustChangePassword: boolean;
  mfaEnabled: boolean;
}
export interface LoginState {
  step: "password" | "bind" | "verify" | "complete";
  expiresAt: string;
  requestId: string;
  redirect?: string;
}
export interface Session {
  user: User;
  administrator: boolean;
  mfaRequired: boolean;
  permissions: string[];
  redirect?: string;
}
export interface Row {
  id: string;
  [key: string]: unknown;
}
export interface List<T = Row> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
export class APIError extends Error {
  constructor(
    public code: string,
    public status: number,
  ) {
    super(code);
  }
}
let csrf: string | undefined;
export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const method = options.method || "GET";
  if (!["GET", "HEAD"].includes(method) && !csrf) {
    const res = await fetch("/api/v1/auth/csrf", {
      credentials: "same-origin",
    });
    if (!res.ok) throw new APIError("SERVICE_UNAVAILABLE", res.status);
    csrf = (await res.json()).token;
  }
  const res = await fetch(path === "/oidc/logout" ? path : `/api/v1${path}`, {
    ...options,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(csrf ? { "X-CSRF-Token": csrf } : {}),
      ...options.headers,
    },
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401 || res.status === 403) csrf = undefined;
    throw new APIError(
      String(data.error?.code || "REQUEST_FAILED").toUpperCase(),
      res.status,
    );
  }
  return data;
}
export const write = <T>(path: string, value: unknown, method = "POST") =>
  api<T>(path, { method, body: JSON.stringify(value) });
export const resetCSRF = () => {
  csrf = undefined;
};
