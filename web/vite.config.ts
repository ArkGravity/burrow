import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: Object.fromEntries(
      ["/api", "/oidc", "/.well-known"].map((p) => [
        p,
        {
          target: "http://127.0.0.1:8080",
          configure(proxy) {
            proxy.on("proxyReq", (req) => {
              req.setHeader("Origin", "http://localhost:8080");
            });
          },
        },
      ]),
    ),
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
