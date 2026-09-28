import { UserManager, WebStorageStateStore } from "oidc-client-ts";
const manager = new UserManager({
  authority: import.meta.env.VITE_OIDC_ISSUER || "http://localhost:8080",
  client_id: import.meta.env.VITE_OIDC_CLIENT_ID || "",
  redirect_uri: "http://localhost:19002/callback",
  post_logout_redirect_uri: "http://localhost:19002/",
  response_type: "code",
  scope: "openid profile email",
  automaticSilentRenew: false,
  monitorSession: false,
  userStore: new WebStorageStateStore({ store: window.sessionStorage }),
});
const output = document.getElementById("result")!;
function report(error: unknown) {
  output.textContent = error instanceof Error ? error.message : String(error);
}
document.getElementById("login")!.onclick = () =>
  void manager.signinRedirect().catch(report);
document.getElementById("logout")!.onclick = () =>
  void manager.signoutRedirect().catch(report);
async function run() {
  if (location.pathname === "/callback") {
    await manager.signinRedirectCallback();
    history.replaceState({}, "", "/");
  }
  const user = await manager.getUser();
  output.textContent = user
    ? JSON.stringify(user.profile, null, 2)
    : "Not signed in";
}
void run().catch(report);
