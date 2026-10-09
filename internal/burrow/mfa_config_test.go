package burrow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	jose "github.com/go-jose/go-jose/v4"
)

func TestMFADisabledLoginAndPolicyChanges(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, admin := testServer(t, driver)
			unbound := mfaUser(t, b, false)
			for _, original := range []User{admin, unbound} {
				b.Config.MFAEnabled = false
				c := newBrowser(b)
				password := testPassword
				if original.ID == admin.ID {
					password += "-changed"
				}
				if step := startLogin(t, c, original.Username, password); step != "complete" {
					t.Fatalf("MFA disabled still requires %s", step)
				}
				if _, ok := c.cookies["burrow_login"]; ok {
					t.Fatal("restricted credential retained after completion")
				}
				w := c.request("GET", "/api/v1/me", nil, false)
				var me struct{ MFARequired bool }
				if err := json.Unmarshal(w.Body.Bytes(), &me); err != nil || w.Code != 200 || me.MFARequired {
					t.Fatalf("password-only session rejected: %d %v", w.Code, err)
				}
				var current User
				if err := b.DB.First(&current, "id = ?", original.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.MFAEnabled != original.MFAEnabled || current.MFACipher != original.MFACipher || current.MFALastStep != original.MFALastStep {
					t.Fatal("disabling verification changed the MFA binding")
				}
				_, session, err := b.session(cookieRequest(c))
				if err != nil || !session.MFAAt.IsZero() {
					t.Fatal("password-only login recorded OTP verification")
				}
				b.Config.MFAEnabled = true
				if c.request("GET", "/api/v1/me", nil, false).Code != 401 {
					t.Fatal("re-enabling MFA accepted password-only session")
				}
				want := "bind"
				if original.MFAEnabled {
					want = "verify"
				}
				if step := startLogin(t, c, original.Username, password); step != want {
					t.Fatalf("re-enabling MFA: got %s want %s", step, want)
				}
				assertRestricted(t, c)
				// A pending MFA login cannot pretend to be complete after a restart
				// with verification disabled; the user must submit their password again.
				b.Config.MFAEnabled = false
				if c.request("GET", "/api/v1/auth/login/status", nil, false).Code != 400 {
					t.Fatal("pending transaction completed without a session")
				}
				if c.request("POST", "/api/v1/auth/login/bind", nil, true).Code != 400 || verifyCode(c, "000000").Code != 400 {
					t.Fatal("MFA endpoints available while disabled")
				}
			}
		})
	}
}

func TestMFADisabledForcedPassword(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, admin := testServer(t, driver)
			b.Config.MFAEnabled = false
			unbound := mfaUser(t, b, true)
			if err := b.DB.Model(&admin).Update("must_change_password", true).Error; err != nil {
				t.Fatal(err)
			}
			for _, u := range []User{unbound, admin} {
				c := newBrowser(b)
				password := testPassword
				if u.ID == admin.ID {
					password += "-changed"
				}
				if step := startLogin(t, c, u.Username, password); step != "password" {
					t.Fatalf("temporary password skipped change: %s", step)
				}
				assertRestricted(t, c)
				w := c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-new"}, true)
				var out struct{ Step, Redirect string }
				if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 || out.Step != "complete" || out.Redirect != "/" {
					t.Fatalf("forced password did not finish: %d %s", w.Code, w.Body.String())
				}
				if c.request("GET", "/api/v1/me", nil, false).Code != 200 {
					t.Fatal("password change did not create valid session")
				}
				if c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword}, true).Code == 200 {
					t.Fatal("completed transaction reused")
				}
			}
		})
	}
}

func TestMFADisabledOIDC(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, kind := range []string{"web", "spa"} {
			t.Run(driver+"/"+kind, func(t *testing.T) {
				b, c, admin := testServer(t, driver)
				b.Config.MFAEnabled = false
				app := testApp(t, b, kind)
				location, _ := authorize(t, c, app, url.Values{"prompt": {"login"}})
				requestID := strings.TrimPrefix(location, "/login?requestId=")
				w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": admin.Username, "password": testPassword + "-changed", "requestId": requestID}, true)
				var out struct{ Step, Redirect string }
				json.Unmarshal(w.Body.Bytes(), &out)
				if w.Code != 200 || out.Step != "complete" || out.Redirect != "/oidc/login?requestId="+requestID {
					t.Fatalf("RP login not continued: %d %s", w.Code, w.Body.String())
				}
				code, verifier := authorize(t, c, app, nil)
				w = exchange(c, app, code, verifier)
				if w.Code != 200 {
					t.Fatalf("exchange: %d %s", w.Code, w.Body.String())
				}
				var tokens map[string]string
				json.Unmarshal(w.Body.Bytes(), &tokens)
				signed, err := jose.ParseSigned(tokens["id_token"], []jose.SignatureAlgorithm{jose.RS256})
				if err != nil {
					t.Fatal(err)
				}
				var key SigningKey
				b.DB.Where("active = ?", true).First(&key)
				var jwk jose.JSONWebKey
				json.Unmarshal([]byte(key.PublicJSON), &jwk)
				payload, err := signed.Verify(jwk.Key)
				if err != nil {
					t.Fatal(err)
				}
				var claims struct {
					AMR      []string `json:"amr"`
					AuthTime int64    `json:"auth_time"`
				}
				json.Unmarshal(payload, &claims)
				if strings.Join(claims.AMR, ",") != "pwd" || claims.AuthTime == 0 {
					t.Fatalf("incorrect password-only claims: %s", payload)
				}
				userinfo := func() int {
					r := httptest.NewRequest("GET", "/oidc/userinfo", nil)
					r.Header.Set("Authorization", "Bearer "+tokens["access_token"])
					w := httptest.NewRecorder()
					b.ServeHTTP(w, r)
					return w.Code
				}
				if userinfo() != 200 {
					t.Fatal("password-only access token rejected")
				}
				if exchange(c, app, code, verifier).Code == 200 {
					t.Fatal("authorization code reused")
				}
				code, verifier = authorize(t, c, app, nil)
				b.Config.MFAEnabled = true
				if exchange(c, app, code, verifier).Code == 200 || userinfo() == 200 {
					t.Fatal("MFA policy change ignored during exchange/UserInfo")
				}
				if location, _ := authorize(t, c, app, url.Values{"prompt": {"none"}}); location != "error:login_required" {
					t.Fatal("password-only session used for SSO with MFA enabled")
				}
			})
		}
	}
}

func TestMFADisabledResetAndSessionBoundaries(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			b.Config.MFAEnabled = false
			startLogin(t, c, admin.Username, testPassword+"-changed")
			u := mfaUser(t, b, false)
			for _, record := range []any{
				&Role{ID: "user-manager", Name: "User manager"},
				&RolePermission{RoleID: "user-manager", PermissionID: "users:write"},
				&UserRole{UserID: u.ID, RoleID: "user-manager"},
			} {
				if err := b.DB.Create(record).Error; err != nil {
					t.Fatal(err)
				}
			}
			ordinary := newBrowser(b)
			startLogin(t, ordinary, u.Username, testPassword)
			reset := map[string]string{"reason": "verified recovery"}
			path := "/api/v1/users/" + admin.ID + "/mfa-reset"
			if ordinary.request("POST", path, reset, true).Code != 403 {
				t.Fatal("ordinary user reset MFA")
			}
			if c.request("POST", path, map[string]string{}, true).Code != 400 {
				t.Fatal("reset without reason accepted")
			}
			if c.request("POST", path, reset, false).Code != 403 {
				t.Fatal("reset without CSRF accepted")
			}
			if w := c.request("POST", path, reset, true); w.Code != 200 {
				t.Fatalf("administrator required OTP with MFA disabled: %d %s", w.Code, w.Body.String())
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 401 {
				t.Fatal("self reset did not revoke password-only session")
			}
			startLogin(t, c, admin.Username, testPassword+"-changed")
			if err := b.DB.Model(&admin).Update("auth_version", admin.AuthVersion+2).Error; err != nil {
				t.Fatal(err)
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 401 {
				t.Fatal("stale auth version accepted with MFA disabled")
			}
			if ordinary.request("POST", "/api/v1/auth/logout", nil, false).Code != 403 {
				t.Fatal("logout without CSRF accepted")
			}
			cookie := *ordinary.cookies["burrow_session"]
			if ordinary.request("POST", "/api/v1/auth/logout", nil, true).Code != 200 {
				t.Fatal("logout failed")
			}
			ordinary.cookies["burrow_session"] = &cookie
			if ordinary.request(http.MethodGet, "/api/v1/me", nil, false).Code != 401 {
				t.Fatal("logout did not revoke session server-side")
			}
		})
	}
}

func TestMFADisabledApplicationAccess(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, _ := testServer(t, driver)
			b.Config.MFAEnabled = false
			u := mfaUser(t, b, true)
			c := newBrowser(b)
			c.init(t)
			app := testApp(t, b, "spa")
			location, _ := authorize(t, c, app, nil)
			requestID := strings.TrimPrefix(location, "/login?requestId=")
			in := map[string]string{"username": u.Username, "password": testPassword, "requestId": requestID}
			if c.request("POST", "/api/v1/auth/login", in, true).Code != 403 {
				t.Fatal("password-only login bypassed application permission")
			}
			for _, record := range []any{
				&Role{ID: "app-access", Name: "App access"},
				&RolePermission{RoleID: "app-access", PermissionID: "app:" + app.ID + ":login"},
				&UserRole{UserID: u.ID, RoleID: "app-access"},
			} {
				if err := b.DB.Create(record).Error; err != nil {
					t.Fatal(err)
				}
			}
			if w := c.request("POST", "/api/v1/auth/login", in, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			assertRestricted(t, c)
			if err := b.DB.Where("user_id = ?", u.ID).Delete(&UserRole{}).Error; err != nil {
				t.Fatal(err)
			}
			if c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true).Code != 403 {
				t.Fatal("password finalization ignored revoked app permission")
			}
			assertRestricted(t, c)
			var current User
			if err := b.DB.First(&current, "id = ?", u.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !current.MustChangePassword || current.PasswordHash != u.PasswordHash {
				t.Fatal("rejected finalization changed password")
			}
		})
	}
}

func TestMFADisabledLoginAuditRollback(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, _ := testServer(t, driver)
			b.Config.MFAEnabled = false
			u := mfaUser(t, b, false)
			c := newBrowser(b)
			c.init(t)
			if err := b.DB.Migrator().DropTable(&Event{}); err != nil {
				t.Fatal(err)
			}
			if c.request("POST", "/api/v1/auth/login", map[string]string{"username": u.Username, "password": testPassword}, true).Code != 503 {
				t.Fatal("unaudited password login succeeded")
			}
			var sessions, pending int64
			b.DB.Model(&Session{}).Where("user_id = ?", u.ID).Count(&sessions)
			b.DB.Model(&LoginTransaction{}).Where("user_id = ?", u.ID).Count(&pending)
			if sessions != 0 || pending != 0 {
				t.Fatal("audit failure did not roll back authentication")
			}
			assertRestricted(t, c)
		})
	}
}
