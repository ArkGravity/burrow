package burrow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAuthorizationRevocationAtExchange(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, _ := testServer(t, driver)
			app := testApp(t, b, "spa")
			u := User{ID: random(18), Username: "member", Enabled: true, LocalEnabled: true}
			u.PasswordHash, _ = passwordHash(testPassword)
			b.DB.Create(&u)
			role := Role{ID: random(18), Name: "application user"}
			b.DB.Create(&role)
			b.DB.Create(&UserRole{UserID: u.ID, RoleID: role.ID})
			grant := RolePermission{RoleID: role.ID, PermissionID: "app:" + app.ID + ":login"}
			b.DB.Create(&grant)
			c := newBrowser(b)
			c.login(t, u.Username, testPassword)
			code, verifier := authorize(t, c, app, nil)
			b.DB.Delete(&grant)
			if w := exchange(c, app, code, verifier); w.Code == 200 {
				t.Fatal("revoked application grant exchanged code")
			}
			code, _ = authorize(t, c, app, nil)
			if code != "error:access_denied" {
				t.Fatalf("missing grant auth response %s", code)
			}
			b.DB.Create(&grant)
			code, verifier = authorize(t, c, app, nil)
			b.DB.Model(&AuthTransaction{}).Where("code_hash = ?", hash(code)).Update("code_expires_at", time.Now().Add(-time.Second))
			if exchange(c, app, code, verifier).Code == 200 {
				t.Fatal("expired code accepted")
			}
		})
	}
}
func TestOIDCLogoutAndCORS(t *testing.T) {
	b, c, _ := testServer(t, "sqlite")
	app := testApp(t, b, "spa")
	for _, endpoint := range []string{"/.well-known/openid-configuration", "/oidc/jwks", "/oidc/userinfo"} {
		r := httptest.NewRequest("OPTIONS", endpoint, nil)
		r.Header.Set("Origin", "http://client.example")
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://client.example" {
			t.Fatalf("SPA cannot fetch %s: %d", endpoint, w.Code)
		}
	}
	code, verifier := authorize(t, c, app, nil)
	w := exchange(c, app, code, verifier)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var tokens map[string]any
	json.Unmarshal(w.Body.Bytes(), &tokens)
	hint := tokens["id_token"].(string)
	w = c.request("GET", "/oidc/logout?id_token_hint="+url.QueryEscape(hint), nil, false)
	if w.Code != 302 || !strings.HasPrefix(w.Header().Get("Location"), "/logout?") {
		t.Fatal("GET logout does not request confirmation")
	}
	if _, _, e := b.session(cookieRequest(c)); e != nil {
		t.Fatal("GET logout revoked session")
	}
	body := map[string]string{"idTokenHint": hint, "postLogoutRedirectUri": "https://evil.example"}
	w = c.request("POST", "/oidc/logout", body, true)
	if w.Code != 400 {
		t.Fatal("logout accepted unregistered redirect")
	}
	body["postLogoutRedirectUri"] = app.LogoutURLs[0]
	body["state"] = "logout-state"
	if w = c.request("POST", "/oidc/logout", body, false); w.Code != 403 {
		t.Fatal("logout missing csrf accepted")
	}
	w = c.request("POST", "/oidc/logout", body, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "logout-state") {
		t.Fatalf("logout %d %s", w.Code, w.Body.String())
	}
	if _, _, e := b.session(cookieRequest(c)); e == nil {
		t.Fatal("logout session still valid")
	}
	for _, origin := range []string{"http://client.example", "https://evil.example"} {
		r := httptest.NewRequest("OPTIONS", "/oidc/token", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		w = httptest.NewRecorder()
		b.ServeHTTP(w, r)
		if origin == "http://client.example" {
			if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != origin {
				t.Fatal("allowed SPA preflight denied")
			}
		} else if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("unregistered CORS origin accepted")
		}
	}
	w = c.request("POST", "/oidc/token", url.Values{"grant_type": {"refresh_token"}}, false)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "unsupported_grant_type") {
		t.Fatal("refresh token grant accepted")
	}
	w = c.request("GET", "/.well-known/openid-configuration", nil, false)
	if strings.Contains(w.Body.String(), "refresh_token") || strings.Contains(w.Body.String(), "offline_access") || strings.Contains(w.Body.String(), "client_secret_post") {
		t.Fatal("discovery advertises unsupported grants")
	}
}
func TestSessionPersistenceAndKeyStartupValidation(t *testing.T) {
	b, c, _ := testServer(t, "sqlite")
	restarted, e := NewServer(b.Store)
	if e != nil {
		t.Fatal(e)
	}
	c.b = restarted
	if w := c.request("GET", "/api/v1/me", nil, false); w.Code != 200 {
		t.Fatal("persisted session rejected after restart")
	}
	b.Store.Config.MasterKey[0] ^= 1
	if _, e = NewServer(b.Store); e == nil {
		t.Fatal("server started with incorrect master key")
	}
}
func TestImmutableJSONFieldCasing(t *testing.T) {
	_, c, admin := testServer(t, "sqlite")
	w := c.request(http.MethodPost, "/api/v1/users", map[string]any{"ID": admin.ID, "username": "attacker", "password": testPassword}, true)
	if w.Code != 400 {
		t.Fatalf("immutable ID casing accepted %d %s", w.Code, w.Body.String())
	}
	w = c.request(http.MethodPost, "/api/v1/roles", map[string]any{"name": "attacker", "Builtin": true}, true)
	if w.Code != 400 {
		t.Fatal("builtin role casing accepted")
	}
}
