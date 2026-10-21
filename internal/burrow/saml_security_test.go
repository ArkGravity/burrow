package burrow

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestSAMLSecurity(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, admin, _ := testServer(t, driver)
			_, config := testSAMLHTTPApp(t, b, admin)
			b.Config.MFAEnabled = false
			user := mfaUser(t, b, false)
			role := Role{ID: random(18), Name: random(18)}
			b.DB.Create(&role)
			b.DB.Create(&UserRole{UserID: user.ID, RoleID: role.ID})
			grant := RolePermission{RoleID: role.ID, PermissionID: "app:" + config.ApplicationID + ":login"}
			b.DB.Create(&grant)
			c := newBrowser(b)
			c.login(t, user.Username, testPassword)
			_, session, err := b.session(cookieRequest(c))
			if err != nil {
				t.Fatal(err)
			}
			pending := func() SAMLTransaction {
				t.Helper()
				now := time.Now()
				st, err := b.registerSAMLRequest(config, "_"+random(12), hash(c.cookies["burrow_browser"].Value), "opaque", false, "", now, now)
				if err != nil {
					t.Fatal(err)
				}
				return st
			}
			for _, tc := range []struct {
				name   string
				mutate func()
				want   int
			}{
				{"user-disabled", func() { b.DB.Model(&user).Update("enabled", false) }, 303},
				{"app-disabled", func() { b.DB.Model(&Application{}).Where("id = ?", config.ApplicationID).Update("enabled", false) }, 400},
				{"auth-version", func() { b.DB.Model(&user).Update("auth_version", user.AuthVersion+1) }, 303},
				{"forced-password", func() { b.DB.Model(&user).Update("must_change_password", true) }, 303},
				{"session-revoked", func() { b.DB.Model(&session).Update("revoked", true) }, 303},
				{"session-expired", func() { b.DB.Model(&session).Update("expires_at", time.Now().UTC().Add(-time.Second)) }, 303},
				{"metadata-changed", func() { b.DB.Model(&config).Update("config_version", config.ConfigVersion+1) }, 400},
				{"mfa-enabled", func() { b.Config.MFAEnabled = true }, 303},
				{"permission-revoked", func() { b.DB.Delete(&grant) }, 200},
			} {
				t.Run(tc.name, func(t *testing.T) {
					st := pending()
					tc.mutate()
					w := c.request("GET", "/saml/login?requestId="+st.ID, nil, false)
					if w.Code != tc.want {
						t.Fatalf("status %d: %s", w.Code, w.Body.String())
					}
					if w.Code == 200 {
						root := testSAMLForm(t, w.Body.String()).Root()
						if root.SelectElement("Assertion") != nil || !strings.HasSuffix(root.SelectElement("Status").SelectElement("StatusCode").SelectElement("StatusCode").SelectAttrValue("Value", ""), ":RequestDenied") {
							t.Fatal("revoked permission issued assertion")
						}
					} else if strings.Contains(w.Body.String(), "SAMLResponse") {
						t.Fatal("unusable session got assertion")
					}
					// Restore only this test's fixture, never external state.
					user.Enabled = true
					user.MustChangePassword = false
					user.AuthVersion = 0
					b.DB.Save(&user)
					session.Revoked = false
					session.ExpiresAt = time.Now().UTC().Add(time.Hour)
					b.DB.Save(&session)
					config.ConfigVersion = 1
					b.DB.Save(&config)
					b.DB.Model(&Application{}).Where("id = ?", config.ApplicationID).Update("enabled", true)
					b.Config.MFAEnabled = false
					b.DB.FirstOrCreate(&grant, grant)
				})
			}
			st := pending()
			other := newBrowser(b)
			other.init(t)
			if w := other.request("GET", "/saml/login?requestId="+st.ID, nil, false); w.Code != 400 {
				t.Fatal("another browser consumed request")
			}
			callback := "test:saml_http_audit_failure"
			b.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "events" {
					tx.AddError(gorm.ErrInvalidData)
				}
			})
			w := c.request("GET", "/saml/login?requestId="+st.ID, nil, false)
			if w.Code != 503 || strings.Contains(w.Body.String(), "SAMLResponse") {
				t.Fatalf("audit failure result: status=%d, form=%t", w.Code, strings.Contains(w.Body.String(), "SAMLResponse"))
			}
			b.DB.Callback().Create().Remove(callback)
			var current SAMLTransaction
			b.DB.First(&current, "id = ?", st.ID)
			if current.Consumed {
				t.Fatal("audit failure consumed request")
			}
			// Independent browsers with copied cookies exercise the same request concurrently.
			var wg sync.WaitGroup
			var forms atomic.Int32
			for range 2 {
				client := newBrowser(b)
				for name, cookie := range c.cookies {
					client.cookies[name] = cookie
				}
				wg.Go(func() {
					if result := client.request("GET", "/saml/login?requestId="+st.ID, nil, false); result.Code == 200 && strings.Contains(result.Body.String(), "SAMLResponse") {
						forms.Add(1)
					}
				})
			}
			wg.Wait()
			if forms.Load() != 1 {
				t.Fatal("concurrent HTTP consume issued more/less than one response")
			}
		})
	}
}

func TestSAMLForceAuthn(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, user := testServer(t, driver)
			key, config := testSAMLHTTPApp(t, b, c)
			b.Config.MFAEnabled = false
			w := c.request("GET", "/saml/sso?"+testSAMLQuery(t, key, testSAMLAuthnXML(b, config, "_force", `ForceAuthn="true"`), ""), nil, false)
			if w.Code != 303 {
				t.Fatal("old session satisfied ForceAuthn")
			}
			location, _ := url.Parse(w.Header().Get("Location"))
			id := location.Query().Get("requestId")
			if w := c.request("GET", "/saml/login?requestId="+id, nil, false); w.Code != 303 {
				t.Fatal("old session accepted on continuation")
			}
			w = c.request("POST", "/api/v1/auth/login", map[string]string{"username": user.Username, "password": testPassword + "-changed", "requestType": "saml", "requestId": id}, true)
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			var result map[string]any
			json.Unmarshal(w.Body.Bytes(), &result)
			if result["redirect"] != "/saml/login?requestId="+id {
				t.Fatal("ForceAuthn login lost request")
			}
			_, sess, err := b.session(cookieRequest(c))
			if err != nil {
				t.Fatal(err)
			}
			if sess.AuthTime.After(time.Now()) {
				t.Fatal("database changed authentication time zone")
			}
			if w := c.request("GET", "/saml/login?requestId="+id, nil, false); w.Code != 200 {
				t.Fatalf("fresh bound session refused: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSAMLRevocation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, user := testServer(t, driver)
			c := newBrowser(b)
			st := testSAMLContinuation(t, b, c, "cancel")
			guard := SAMLRequestReplay{SPEntityID: "https://sp.test", SPRequestID: st.SPRequestID, ReplayUntil: time.Now().Add(time.Minute)}
			b.DB.Create(&guard)
			w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": user.Username, "password": testPassword + "-changed", "requestType": "saml", "requestId": st.ID}, true)
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if w := c.request("POST", "/api/v1/auth/login/cancel", map[string]any{}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			var count int64
			b.DB.Model(&SAMLTransaction{}).Where("id = ?", st.ID).Count(&count)
			if count != 0 {
				t.Fatal("cancel did not remove pending SAML request")
			}
			b.DB.Model(&SAMLRequestReplay{}).Where("sp_entity_id = ?", guard.SPEntityID).Count(&count)
			if count != 1 {
				t.Fatal("cancel deleted replay guard")
			}
			st = testSAMLContinuation(t, b, c, "reset")
			if w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": user.Username, "password": testPassword + "-changed", "requestType": "saml", "requestId": st.ID}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if err := b.ResetMFA(user.Username, "test reset"); err != nil {
				t.Fatal(err)
			}
			b.DB.Model(&SAMLTransaction{}).Where("id = ?", st.ID).Count(&count)
			if count != 0 {
				t.Fatal("reset retained pending SAML request")
			}
		})
	}
}
