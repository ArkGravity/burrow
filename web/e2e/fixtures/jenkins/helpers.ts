import { request as httpsRequest } from "node:https";
import { createHmac } from "node:crypto";

export type APIState = { cookies: Record<string, string>; csrf?: string };
export async function call(
  host: string,
  path: string,
  method = "GET",
  data?: unknown,
  state: APIState = { cookies: {} },
  form = false,
) {
  const raw =
    data === undefined ? undefined : form ? String(data) : JSON.stringify(data);
  return new Promise<{
    status: number;
    body: string;
    json: any;
    location?: string;
  }>((resolve, reject) => {
    const request = httpsRequest(
      {
        hostname: "127.0.0.1",
        port: Number(process.env.BURROW_SAML_HTTPS_PORT),
        path,
        method,
        rejectUnauthorized: false,
        headers: {
          Host: `${host}:${process.env.BURROW_SAML_HTTPS_PORT}`,
          Origin: `https://sso.burrow.test:${process.env.BURROW_SAML_HTTPS_PORT}`,
          Cookie: Object.entries(state.cookies)
            .map(([key, value]) => `${key}=${value}`)
            .join("; "),
          "Content-Type": form
            ? "application/x-www-form-urlencoded"
            : "application/json",
          ...(state.csrf ? { "X-CSRF-Token": state.csrf } : {}),
        },
      },
      (response) => {
        for (const value of response.headers["set-cookie"] || []) {
          const pair = value.split(";", 1)[0]!;
          const separator = pair.indexOf("=");
          if (separator > 0)
            state.cookies[pair.slice(0, separator)] = pair.slice(separator + 1);
        }
        const chunks: Buffer[] = [];
        response.on("data", (chunk) => chunks.push(chunk));
        response.on("end", () => {
          const body = Buffer.concat(chunks).toString("utf8");
          let json: any;
          try {
            json = JSON.parse(body);
          } catch {}
          resolve({
            status: response.statusCode || 0,
            body,
            json,
            location: response.headers.location,
          });
        });
      },
    );
    request.setTimeout(10000, () =>
      request.destroy(new Error("fixture HTTP timeout")),
    );
    request.on("error", reject);
    if (raw !== undefined) request.write(raw);
    request.end();
  });
}

const usedSteps = new Map<string, number>();
export async function nextCode(secret: string) {
  let step = Math.floor(Date.now() / 30000);
  if ((usedSteps.get(secret) ?? -1) >= step) {
    await new Promise((resolve) =>
      setTimeout(resolve, (step + 1) * 30000 - Date.now() + 100),
    );
    step = Math.floor(Date.now() / 30000);
  }
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  const bits = [...secret]
    .map((char) => alphabet.indexOf(char).toString(2).padStart(5, "0"))
    .join("");
  const key = Buffer.from(
    bits.match(/.{8}/g)!.map((value) => parseInt(value, 2)),
  );
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(step));
  const digest = createHmac("sha1", key).update(counter).digest();
  const offset = digest[digest.length - 1]! & 15;
  usedSteps.set(secret, step);
  return String((digest.readUInt32BE(offset) & 0x7fffffff) % 1000000).padStart(
    6,
    "0",
  );
}
