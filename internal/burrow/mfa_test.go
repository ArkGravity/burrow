package burrow

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestTOTPStandardVectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	// Six-digit suffixes of the SHA1 reference vectors in RFC 6238, Appendix B.
	for _, v := range []struct {
		timestamp int64
		code      string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		code, err := otpCode(secret, v.timestamp/30)
		if err != nil || code != v.code {
			t.Fatalf("RFC vector %d: %s %v", v.timestamp, code, err)
		}
	}
	now := time.Unix(1234567890, 0)
	for _, offset := range []int64{-1, 0, 1} {
		step := now.Unix()/30 + offset
		code, _ := otpCode(secret, step)
		if got, ok := otpStep(secret, code, now, step-1); !ok || got != step {
			t.Fatal("skew window rejected")
		}
		if _, ok := otpStep(secret, code, now, step); ok {
			t.Fatal("replayed step accepted")
		}
	}
	code, _ := otpCode(secret, now.Unix()/30+2)
	if _, ok := otpStep(secret, code, now, -1); ok {
		t.Fatal("wide window accepted")
	}
	for _, invalid := range []string{"", "12345", "1234567", "１２３４５６", "12 456"} {
		if _, ok := otpStep(secret, invalid, now, -1); ok {
			t.Fatal("invalid format")
		}
	}
}
func startLogin(t *testing.T, c *browser, username, password string) string {
	t.Helper()
	c.init(t)
	w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": username, "password": password}, true)
	if w.Code != 200 {
		t.Fatalf("password login %d: %s", w.Code, w.Body.String())
	}
	var out struct{ Step string }
	json.Unmarshal(w.Body.Bytes(), &out)
	return out.Step
}
func bindingSecret(t *testing.T, c *browser) string {
	t.Helper()
	w := c.request("POST", "/api/v1/auth/login/bind", map[string]string{}, true)
	var out struct{ Secret, URI string }
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Secret == "" || !strings.HasPrefix(out.URI, "otpauth://totp/") {
		t.Fatal(w.Body.String())
	}
	return out.Secret
}
func codeFor(t *testing.T, b *Server, u User) string {
	t.Helper()
	b.DB.First(&u, "id = ?", u.ID)
	secret, err := b.unseal(u.MFACipher)
	if err != nil {
		t.Fatal(err)
	}
	now := b.mfaTime()
	if now.Unix()/30 <= u.MFALastStep {
		now = time.Unix((u.MFALastStep+1)*30, 0)
	}
	b.mfaTime = func() time.Time { return now }
	code, err := otpCode(secret, now.Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	return code
}
func verifyCode(c *browser, code string) *httptest.ResponseRecorder {
	return c.request("POST", "/api/v1/auth/login/verify", map[string]string{"code": code}, true)
}
func assertRestricted(t *testing.T, c *browser) {
	t.Helper()
	for _, endpoint := range []string{"/api/v1/me", "/api/v1/me/apps", "/api/v1/users", "/api/v1/dashboard"} {
		if w := c.request("GET", endpoint, nil, false); w.Code != 401 {
			t.Fatalf("restricted access %s: %d", endpoint, w.Code)
		}
	}
	if _, ok := c.cookies["burrow_session"]; ok {
		t.Fatal("formal cookie issued before completion")
	}
}
func mfaUser(t *testing.T, b *Server, temp bool) User {
	t.Helper()
	h, err := passwordHash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	u := User{ID: random(18), Username: random(9), PasswordHash: h, Enabled: true, MustChangePassword: temp, Language: "en", Theme: "system"}
	if err := b.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}
func TestMandatoryMFAFlows(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, adminBrowser, admin := testServer(t, driver)
			u := mfaUser(t, b, true)
			c := newBrowser(b)
			if startLogin(t, c, u.Username, testPassword) != "password" {
				t.Fatal("temporary unbound order")
			}
			assertRestricted(t, c)
			if c.request("POST", "/api/v1/auth/login/bind", nil, true).Code != 400 {
				t.Fatal("bound before changing password")
			}
			old := c.cookies["burrow_login"].Value
			if w := c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if c.cookies["burrow_login"].Value == old {
				t.Fatal("restricted cookie not rotated")
			}
			assertRestricted(t, c)
			secret := bindingSecret(t, c)
			if bindingSecret(t, c) != secret {
				t.Fatal("refresh replaced pending secret")
			}
			if w := c.request("GET", "/api/v1/auth/login/status", nil, false); w.Code != 200 {
				t.Fatal("cannot restore step")
			}
			var l LoginTransaction
			b.DB.Where("user_id = ?", u.ID).First(&l)
			if l.PendingCipher == secret || strings.Contains(l.PendingCipher, secret) {
				t.Fatal("pending secret plaintext")
			}
			code, _ := otpCode(secret, b.mfaTime().Unix()/30)
			if verifyCode(c, code).Code != 200 {
				t.Fatal("enrollment failed")
			}
			if verifyCode(c, code).Code == 200 {
				t.Fatal("transaction consumed twice")
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 200 {
				t.Fatal("no formal session")
			}
			var current User
			b.DB.First(&current, "id = ?", u.ID)
			if !current.MFAEnabled || current.MFACipher == secret || current.MustChangePassword {
				t.Fatal("invalid completed state")
			}
			list := adminBrowser.request("GET", "/api/v1/users", nil, false)
			if strings.Contains(list.Body.String(), secret) || strings.Contains(list.Body.String(), current.MFACipher) {
				t.Fatal("secret disclosed in list")
			}
			c.request("POST", "/api/v1/auth/logout", nil, true)
			if startLogin(t, c, u.Username, testPassword+"-changed") != "verify" {
				t.Fatal("bound account did not require OTP")
			}
			assertRestricted(t, c)
			if verifyCode(c, code).Code != 400 {
				t.Fatal("enrollment code reused for login")
			}
			c.verifyMFA(t)
			// Password reset retains the original authenticator and verifies it before changing password.
			if w := adminBrowser.request("PUT", "/api/v1/users/"+u.ID+"/password", map[string]string{"password": testPassword}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 401 {
				t.Fatal("password reset left session")
			}
			if startLogin(t, c, u.Username, testPassword) != "verify" {
				t.Fatal("reset password bypassed original MFA")
			}
			if c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true).Code != 400 {
				t.Fatal("changed reset password before MFA")
			}
			c.verifyMFA(t)
			assertRestricted(t, c)
			w := c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true)
			if w.Code != 200 || c.request("GET", "/api/v1/me", nil, false).Code != 200 {
				t.Fatal("bound forced change failed")
			}
			// MFA reset keeps the password; the former session and pending login both become invalid.
			pending := newBrowser(b)
			startLogin(t, pending, u.Username, testPassword+"-changed")
			if w := adminBrowser.request("POST", "/api/v1/users/"+u.ID+"/mfa-reset", map[string]string{"code": codeFor(t, b, admin), "reason": "lost phone"}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 401 || pending.request("GET", "/api/v1/auth/login/status", nil, false).Code == 200 {
				t.Fatal("MFA reset left access")
			}
			if startLogin(t, c, u.Username, testPassword+"-changed") != "bind" {
				t.Fatal("MFA reset changed password or did not force binding")
			}
			rebound := bindingSecret(t, c)
			if rebound == secret {
				t.Fatal("old secret reused")
			}
			c.verifyMFA(t)
		})
	}
}
func TestMFAExpiryAttemptsAndBrowserBinding(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, _ := testServer(t, driver)
			u := mfaUser(t, b, false)
			c := newBrowser(b)
			startLogin(t, c, u.Username, testPassword)
			secret := bindingSecret(t, c)
			other := newBrowser(b)
			other.init(t)
			other.cookies["burrow_login"] = c.cookies["burrow_login"]
			if other.request("GET", "/api/v1/auth/login/status", nil, false).Code == 200 {
				t.Fatal("different browser consumed transaction")
			}
			for i := 0; i < 5; i++ {
				if verifyCode(c, "wrong").Code != 400 {
					t.Fatal("invalid code attempts")
				}
			}
			code, _ := otpCode(secret, b.mfaTime().Unix()/30)
			if verifyCode(c, code).Code == 200 {
				t.Fatal("attempt limit bypass")
			}
			assertRestricted(t, c)
			startLogin(t, c, u.Username, testPassword)
			b.DB.Model(&LoginTransaction{}).Where("user_id = ?", u.ID).Update("expires_at", time.Now().Add(-time.Second))
			if c.request("POST", "/api/v1/auth/login/bind", nil, true).Code == 200 {
				t.Fatal("expired transaction bound")
			}
			startLogin(t, c, u.Username, testPassword)
			bindingSecret(t, c)
			if c.request("POST", "/api/v1/auth/login/cancel", nil, true).Code != 200 {
				t.Fatal("cancel failed")
			}
			if c.request("GET", "/api/v1/auth/login/status", nil, false).Code == 200 {
				t.Fatal("cancelled login valid")
			}
			startLogin(t, c, u.Username, testPassword)
			b.DB.Model(&User{}).Where("id = ?", u.ID).Update("auth_version", gorm.Expr("auth_version + 1"))
			if c.request("POST", "/api/v1/auth/login/bind", nil, true).Code == 200 {
				t.Fatal("stale auth version accepted")
			}
		})
	}
}

func TestMFAAccountLimitSurvivesNewTransactions(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, _ := testServer(t, driver)
			u := mfaUser(t, b, false)
			c := newBrowser(b)
			for transaction := 0; transaction < 3; transaction++ {
				startLogin(t, c, u.Username, testPassword)
				bindingSecret(t, c)
				for attempt := 0; attempt < 4; attempt++ {
					expected := 400
					if transaction*4+attempt >= 10 {
						expected = 429
					}
					if w := verifyCode(c, "wrong"); w.Code != expected {
						t.Fatalf("new login bypassed account MFA limit: %d", w.Code)
					}
				}
			}
			assertRestricted(t, c)
		})
	}
}
func TestRestrictedPasswordPreservesDeadlineAndRevokesCredential(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, _ := testServer(t, driver)
			u := mfaUser(t, b, true)
			c := newBrowser(b)
			startLogin(t, c, u.Username, testPassword)
			old := newBrowser(b)
			old.csrf = c.csrf
			for name, cookie := range c.cookies {
				old.cookies[name] = cookie
			}
			var before LoginTransaction
			b.DB.Where("user_id = ?", u.ID).First(&before)
			if w := c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			var after LoginTransaction
			b.DB.Where("user_id = ?", u.ID).First(&after)
			if !before.ExpiresAt.Equal(after.ExpiresAt) || before.CredentialHash == after.CredentialHash || before.AuthVersion >= after.AuthVersion {
				t.Fatal("password change extended or did not rotate transaction")
			}
			if old.request("GET", "/api/v1/auth/login/status", nil, false).Code == 200 {
				t.Fatal("old restricted credential valid")
			}
			assertRestricted(t, c)
			anon := newBrowser(b)
			anon.init(t)
			if anon.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword}, true).Code != 400 {
				t.Fatal("anonymous restricted password change accepted")
			}
		})
	}
}
func TestMFASingleConcurrentUse(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, admin := testServer(t, driver)
			a, c := newBrowser(b), newBrowser(b)
			startLogin(t, a, admin.Username, testPassword+"-changed")
			startLogin(t, c, admin.Username, testPassword+"-changed")
			code := codeFor(t, b, admin)
			var wg sync.WaitGroup
			statuses := make(chan int, 2)
			for _, client := range []*browser{a, c} {
				wg.Go(func() { statuses <- verifyCode(client, code).Code })
			}
			wg.Wait()
			close(statuses)
			successes := 0
			for status := range statuses {
				if status == 200 {
					successes++
				}
			}
			if successes != 1 {
				t.Fatalf("concurrent OTP accepted %d times", successes)
			}
			// Two pending secrets cannot overwrite each other after one enrollment wins.
			u := mfaUser(t, b, false)
			a, c = newBrowser(b), newBrowser(b)
			startLogin(t, a, u.Username, testPassword)
			startLogin(t, c, u.Username, testPassword)
			sa, sc := bindingSecret(t, a), bindingSecret(t, c)
			ca, _ := otpCode(sa, b.mfaTime().Unix()/30)
			cc, _ := otpCode(sc, b.mfaTime().Unix()/30)
			if verifyCode(a, ca).Code != 200 {
				t.Fatal("first binding failed")
			}
			if verifyCode(c, cc).Code == 200 {
				t.Fatal("second pending secret replaced binding")
			}
			var current User
			b.DB.First(&current, "id = ?", u.ID)
			saved, _ := b.unseal(current.MFACipher)
			if saved != sa {
				t.Fatal("binding overwritten")
			}
		})
	}
}
func TestMFAResetPermissionsAndAuditRollback(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			u := mfaUser(t, b, false)
			targetBrowser := newBrowser(b)
			targetBrowser.login(t, u.Username, testPassword)
			b.DB.First(&u, "id = ?", u.ID)
			manager := mfaUser(t, b, false)
			b.DB.Create(&Role{ID: "manager", Name: "Manager"})
			b.DB.Create(&RolePermission{RoleID: "manager", PermissionID: "users:write"})
			b.DB.Create(&UserRole{UserID: manager.ID, RoleID: "manager"})
			limited := newBrowser(b)
			limited.login(t, manager.Username, testPassword)
			if limited.request("POST", "/api/v1/users/"+u.ID+"/mfa-reset", map[string]string{"code": codeFor(t, b, manager), "reason": "attempt"}, true).Code != 403 {
				t.Fatal("users:write reset MFA")
			}
			if c.request("POST", "/api/v1/users/"+u.ID+"/mfa-reset", map[string]string{"code": "000000", "reason": ""}, true).Code != 400 {
				t.Fatal("reason optional")
			}
			code := codeFor(t, b, admin)
			if err := b.DB.Migrator().DropTable(&Event{}); err != nil {
				t.Fatal(err)
			}
			if c.request("POST", "/api/v1/users/"+u.ID+"/mfa-reset", map[string]string{"code": code, "reason": "lost phone"}, true).Code == 200 {
				t.Fatal("reset without audit")
			}
			var current User
			b.DB.First(&current, "id = ?", u.ID)
			if current.MFACipher != u.MFACipher || current.AuthVersion != u.AuthVersion || targetBrowser.request("GET", "/api/v1/me", nil, false).Code != 200 {
				t.Fatal("unaudited reset committed")
			}
			if b.ResetMFA(u.Username, "operator recovery") == nil {
				t.Fatal("CLI reset without audit")
			}
			b.DB.First(&current, "id = ?", u.ID)
			if !current.MFAEnabled {
				t.Fatal("CLI audit failure did not roll back")
			}
		})
	}
}
func TestMFAOperatorRecoveryAndOIDCRevocation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			app := testApp(t, b, "spa")
			code, verifier := authorize(t, c, app, nil)
			response := exchange(c, app, code, verifier)
			var token struct {
				AccessToken string `json:"access_token"`
			}
			json.Unmarshal(response.Body.Bytes(), &token)
			if response.Code != 200 {
				t.Fatal(response.Body.String())
			}
			outstanding, v := authorize(t, c, app, nil)
			pending := newBrowser(b)
			startLogin(t, pending, admin.Username, testPassword+"-changed")
			if err := b.ResetMFA(admin.Username, "lost administrator authenticator"); err != nil {
				t.Fatal(err)
			}
			if exchange(c, app, outstanding, v).Code == 200 {
				t.Fatal("old code exchanged after reset")
			}
			r := httptest.NewRequest(http.MethodGet, "/oidc/userinfo", nil)
			r.Header.Set("Authorization", "Bearer "+token.AccessToken)
			w := httptest.NewRecorder()
			b.ServeHTTP(w, r)
			if w.Code == 200 {
				t.Fatal("old access token accepted")
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 401 || pending.request("GET", "/api/v1/auth/login/status", nil, false).Code == 200 {
				t.Fatal("operator reset left authentication")
			}
			if startLogin(t, c, admin.Username, testPassword+"-changed") != "bind" {
				t.Fatal("operator reset did not force enrollment")
			}
			c.verifyMFA(t)
			if c.request("GET", "/api/v1/users", nil, false).Code != 200 {
				t.Fatal("last admin not recoverable")
			}
			u := mfaUser(t, b, false)
			b.DB.Model(&User{}).Where("id = ?", u.ID).Update("enabled", false)
			if err := b.ResetMFA(u.Username, "disabled user recovery"); err != nil {
				t.Fatal(err)
			}
			b.DB.First(&u, "id = ?", u.ID)
			if u.Enabled {
				t.Fatal("CLI enabled user")
			}
			var event Event
			if b.DB.Where("kind = ? AND actor_id = ? AND object_id = ?", "mfa:reset", "operator:cli", admin.ID).First(&event).Error != nil || !strings.Contains(event.Details, "authenticator") {
				t.Fatal("operator audit missing")
			}
		})
	}
}
func TestMFAMigrationUpgrade(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, rollback := range []bool{false, true} {
			t.Run(driver+map[bool]string{false: "/upgrade", true: "/rollback"}[rollback], func(t *testing.T) {
				s := testLegacyStore(t, driver, 4)
				h, _ := passwordHash(testPassword)
				if err := s.DB.Table("users").Create(map[string]any{"id": "legacy", "username": "legacy", "password_hash": h, "enabled": true, "must_change_password": false, "language": "zh-CN", "theme": "dark"}).Error; err != nil {
					t.Fatal(err)
				}
				session := Session{ID: "legacy", UserID: "legacy", CredentialHash: hash("legacy"), Method: "password", AuthTime: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
				if err := s.DB.Omit("MFAAt", "AuthVersion").Create(&session).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.DB.Create(&Application{ID: "legacy", ClientID: "legacy", Enabled: true}).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.DB.Create(&TokenRecord{ID: "legacy", UserID: "legacy", ClientID: "legacy", SessionID: "legacy", ExpiresAt: session.ExpiresAt}).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.DB.Create(&AuthTransaction{ID: "legacy", UserID: "legacy", ClientID: "legacy", SessionID: "legacy", ExpiresAt: session.ExpiresAt}).Error; err != nil {
					t.Fatal(err)
				}
				if rollback {
					s.DB.Migrator().DropTable(&Event{})
				}
				err := s.Migrate()
				if rollback {
					if err == nil {
						t.Fatal("upgrade without audit")
					}
					var v SchemaVersion
					s.DB.First(&v, 1)
					if v.Version != 4 || s.DB.Migrator().HasColumn("users", "mfa_enabled") || s.DB.Migrator().HasTable(&LoginTransaction{}) {
						t.Fatal("failed MFA upgrade changed schema")
					}
					s.DB.First(&session, "id = ?", "legacy")
					if session.Revoked {
						t.Fatal("failed migration revoked session")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = s.Migrate(); err != nil {
					t.Fatal(err)
				}
				if err = s.Health(context.Background()); err != nil {
					t.Fatal(err)
				}
				var u User
				s.DB.First(&u, "id = ?", "legacy")
				if u.MFAEnabled || u.MFACipher != "" || u.PasswordHash != h || !u.Enabled || u.Language != "zh-CN" || u.Theme != "dark" {
					t.Fatal("upgrade changed identity data")
				}
				s.DB.First(&session, "id = ?", "legacy")
				if !session.Revoked {
					t.Fatal("non-MFA session survived")
				}
				var token TokenRecord
				s.DB.First(&token, "id = ?", "legacy")
				if !token.Revoked {
					t.Fatal("old token survived")
				}
				var n int64
				s.DB.Model(&AuthTransaction{}).Count(&n)
				if n != 0 {
					t.Fatal("old authorization survived")
				}
			})
		}
	}
}
func TestMFAOIDCFreshAuthentication(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			app := testApp(t, b, "spa")
			location, _ := authorize(t, c, app, url.Values{"prompt": {"login"}})
			if !strings.HasPrefix(location, "/login?") {
				t.Fatal("fresh authentication skipped")
			}
			for _, options := range []url.Values{{"prompt": {"login"}, "max_age": {"3600"}}, {"max_age": {"0"}}} {
				fresh, _ := authorize(t, c, app, options)
				if !strings.HasPrefix(fresh, "/login?") {
					t.Fatal("max_age bypassed fresh authentication")
				}
			}
			requestID := strings.TrimPrefix(location, "/login?requestId=")
			w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": admin.Username, "password": testPassword + "-changed", "requestId": requestID}, true)
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			assertRestricted(t, c)
			none, _ := authorize(t, c, app, url.Values{"prompt": {"none"}})
			if none != "error:login_required" {
				t.Fatal("partial login used for SSO")
			}
			if c.request("GET", "/oidc/login?requestId="+requestID, nil, false).Header().Get("Location") == "" {
				t.Fatal("partial request not returned to login")
			}
			c.verifyMFA(t)
			w = c.request("GET", "/oidc/login?requestId="+requestID, nil, false)
			if !strings.HasPrefix(w.Header().Get("Location"), "/oidc/authorize/callback") {
				t.Fatal("fresh password+OTP did not resume RP")
			}
			var a AuthTransaction
			b.DB.First(&a, "id = ?", requestID)
			loaded, err := b.authenticatedAuth(b.DB, a)
			if err != nil || strings.Join(loaded.GetAMR(), ",") != "pwd,otp" || loaded.GetAuthTime().IsZero() {
				t.Fatal("incorrect AMR/auth_time")
			}
			// A session without MFA or with a stale version cannot issue codes or access APIs.
			var session Session
			b.DB.Where("credential_hash = ?", hash(c.cookies["burrow_session"].Value)).First(&session)
			b.DB.Model(&session).Update("auth_version", session.AuthVersion+1)
			if c.request("GET", "/api/v1/me", nil, false).Code != 401 {
				t.Fatal("stale session version accepted")
			}
			none, _ = authorize(t, c, app, url.Values{"prompt": {"none"}})
			if none != "error:login_required" {
				t.Fatal("stale MFA session used for OIDC")
			}
		})
	}
}

func TestMFAFinalizationRechecksAndAudit(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, scenario := range []string{"disabled", "password-reset", "audit", "request-expired", "app-disabled", "grant-revoked"} {
			t.Run(driver+"/"+scenario, func(t *testing.T) {
				b, actor, _ := testServer(t, driver)
				u := mfaUser(t, b, false)
				c := newBrowser(b)
				c.init(t)
				app := testApp(t, b, "spa")
				b.DB.Create(&Role{ID: "app-login", Name: "App login"})
				b.DB.Create(&RolePermission{RoleID: "app-login", PermissionID: "app:" + app.ID + ":login"})
				b.DB.Create(&UserRole{RoleID: "app-login", UserID: u.ID})
				location, _ := authorize(t, c, app, nil)
				id := strings.TrimPrefix(location, "/login?requestId=")
				w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": u.Username, "password": testPassword, "requestId": id}, true)
				if w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				secret := bindingSecret(t, c)
				code, _ := otpCode(secret, b.mfaTime().Unix()/30)
				switch scenario {
				case "disabled":
					b.DB.Model(&User{}).Where("id = ?", u.ID).Update("enabled", false)
				case "password-reset":
					if w := actor.request("PUT", "/api/v1/users/"+u.ID+"/password", map[string]string{"password": testPassword + "-reset"}, true); w.Code != 200 {
						t.Fatal(w.Body.String())
					}
				case "audit":
					b.DB.Migrator().DropTable(&Event{})
				case "request-expired":
					b.DB.Model(&AuthTransaction{}).Where("id = ?", id).Update("expires_at", time.Now().Add(-time.Second))
				case "app-disabled":
					b.DB.Model(&Application{}).Where("id = ?", app.ID).Update("enabled", false)
				case "grant-revoked":
					b.DB.Where("role_id = ?", "app-login").Delete(&RolePermission{})
				}
				if verifyCode(c, code).Code == 200 {
					t.Fatal("invalid or unaudited binding completed")
				}
				var current User
				b.DB.First(&current, "id = ?", u.ID)
				if current.MFAEnabled || current.MFACipher != "" {
					t.Fatal("failed finalization stored secret")
				}
				assertRestricted(t, c)
			})
		}
	}
}
func TestMFAResetRechecksActorAndConsumesCode(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			u := mfaUser(t, b, false)
			target := newBrowser(b)
			target.login(t, u.Username, testPassword)
			originalCode := codeFor(t, b, admin)
			if w := c.request("POST", "/api/v1/users/"+u.ID+"/mfa-reset", map[string]string{"code": originalCode, "reason": "verified reset"}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if w := c.request("POST", "/api/v1/users/"+u.ID+"/mfa-reset", map[string]string{"code": originalCode, "reason": "replay"}, true); w.Code != 400 {
				t.Fatal("admin code reused")
			}
			fresh := codeFor(t, b, admin)
			body, _ := json.Marshal(map[string]string{"code": fresh, "reason": "stale actor"})
			w := interleave(c, "POST", "/api/v1/users/"+u.ID+"/mfa-reset", string(body), func() { b.DB.Where("user_id = ? AND role_id = ?", admin.ID, "admin").Delete(&UserRole{}) })
			if w.Code != 403 {
				t.Fatal("demoted actor reset MFA")
			}
			var current User
			b.DB.First(&current, "id = ?", admin.ID)
			if current.MFALastStep == b.mfaTime().Unix()/30 {
				t.Fatal("forbidden reset consumed OTP")
			}
		})
	}
}
