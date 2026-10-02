package burrow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

const testPassword = "test-password-12345"

type browser struct {
	b       *Server
	cookies map[string]*http.Cookie
	csrf    string
}

func newBrowser(b *Server) *browser { return &browser{b: b, cookies: map[string]*http.Cookie{}} }
func cookieRequest(c *browser) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range c.cookies {
		r.AddCookie(cookie)
	}
	return r
}
func (c *browser) request(method, path string, body any, csrf bool) *httptest.ResponseRecorder {
	var raw []byte
	content := "application/json"
	switch v := body.(type) {
	case url.Values:
		raw = []byte(v.Encode())
		content = "application/x-www-form-urlencoded"
	case nil:
	default:
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Content-Type", content)
	for _, cookie := range c.cookies {
		r.AddCookie(cookie)
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", c.csrf)
	}
	w := httptest.NewRecorder()
	c.b.ServeHTTP(w, r)
	for _, cookie := range w.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
		} else {
			c.cookies[cookie.Name] = cookie
		}
	}
	return w
}
func (c *browser) init(t *testing.T) {
	t.Helper()
	w := c.request("GET", "/api/v1/auth/csrf", nil, false)
	var v map[string]string
	json.Unmarshal(w.Body.Bytes(), &v)
	c.csrf = v["token"]
	if c.csrf == "" {
		t.Fatal(w.Body.String())
	}
}
func (c *browser) login(t *testing.T, username, password string) {
	t.Helper()
	c.init(t)
	w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": username, "password": password}, true)
	if w.Code != 200 {
		t.Fatalf("login %d: %s", w.Code, w.Body.String())
	}
	var result struct{ Step string }
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Step == "verify" || result.Step == "bind" {
		c.verifyMFA(t)
	}
}
func (c *browser) verifyMFA(t *testing.T) {
	t.Helper()
	u, l, err := c.b.loginStateDB(c.b.DB, cookieRequest(c))
	if err != nil {
		t.Fatal(err)
	}
	secret := ""
	last := int64(-1)
	if loginStep(u, l) == "bind" {
		w := c.request("POST", "/api/v1/auth/login/bind", map[string]string{}, true)
		var result struct{ Secret string }
		json.Unmarshal(w.Body.Bytes(), &result)
		if w.Code != 200 || result.Secret == "" {
			t.Fatal(w.Body.String())
		}
		secret = result.Secret
	} else {
		secret, err = c.b.unseal(u.MFACipher)
		if err != nil {
			t.Fatal(err)
		}
		last = u.MFALastStep
	}
	// Advance the injected verifier clock, avoiding real-time sleeps in unrelated tests.
	now := c.b.mfaTime()
	if now.Unix()/30 <= last {
		now = time.Unix((last+1)*30, 0)
	}
	c.b.mfaTime = func() time.Time { return now }
	code, err := otpCode(secret, now.Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	w := c.request("POST", "/api/v1/auth/login/verify", map[string]string{"code": code}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
func openTestStore(t *testing.T, driver string) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	if driver == "postgres" {
		dsn = os.Getenv("BURROW_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("BURROW_TEST_POSTGRES_DSN not configured")
		}
	}
	c := Config{Env: "dev", Issuer: "http://localhost:8080", DBDriver: driver, DBDSN: dsn}
	copy(c.MasterKey[:], []byte("01234567890123456789012345678901"))
	s, e := Open(c)
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := s.DB.DB()
	t.Cleanup(func() { sql.Close() })
	if driver == "postgres" {
		schema := "burrow_test_" + strings.ReplaceAll(random(9), "-", "_")
		if e = s.DB.Exec("CREATE SCHEMA " + schema).Error; e != nil {
			t.Fatal(e)
		}
		scoped := c
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			u, e := url.Parse(dsn)
			if e != nil {
				t.Fatal(e)
			}
			q := u.Query()
			q.Set("search_path", schema)
			u.RawQuery = q.Encode()
			scoped.DBDSN = u.String()
		} else {
			scoped.DBDSN = dsn + " search_path=" + schema
		}
		isolated, e := Open(scoped)
		if e != nil {
			t.Fatal(e)
		}
		s = isolated
		pool, _ := s.DB.DB()
		t.Cleanup(func() { pool.Close() })
		t.Cleanup(func() { s.DB.Exec("DROP SCHEMA " + schema + " CASCADE") })
	}
	return s
}
func testStore(t *testing.T, driver string) *Store {
	t.Helper()
	s := openTestStore(t, driver)
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s
}
func testServer(t *testing.T, driver string) (*Server, *browser, User) {
	t.Helper()
	s := testStore(t, driver)
	if e := s.RotateKeys(); e != nil {
		t.Fatal(e)
	}
	if e := s.Seed(BootstrapConfig{Username: "admin", Password: testPassword}); e != nil {
		t.Fatal(e)
	}
	b, e := NewServer(s)
	if e != nil {
		t.Fatal(e)
	}
	c := newBrowser(b)
	c.login(t, "admin", testPassword)
	w := c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	c.verifyMFA(t)
	var u User
	s.DB.First(&u, "username = ?", "admin")
	return b, c, u
}
func TestMigrationIntegrity(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := testStore(t, driver)
			if e := s.Migrate(); e != nil {
				t.Fatal(e)
			}
			if e := s.Health(context.Background()); e != nil {
				t.Fatal(e)
			}
			if e := s.DB.Create(&UserRole{UserID: "missing", RoleID: "admin"}).Error; e == nil {
				t.Fatal("foreign key accepted missing user")
			}
			s.DB.Model(&SchemaVersion{}).Where("id = ?", 1).Update("checksum", "tampered")
			if s.Migrate() == nil || s.Health(context.Background()) == nil {
				t.Fatal("checksum mismatch accepted")
			}
		})
	}
}
func TestCSRFAndForcedPassword(t *testing.T) {
	s := testStore(t, "sqlite")
	if e := s.RotateKeys(); e != nil {
		t.Fatal(e)
	}
	if e := s.Seed(BootstrapConfig{Username: "admin", Password: testPassword}); e != nil {
		t.Fatal(e)
	}
	b, e := NewServer(s)
	if e != nil {
		t.Fatal(e)
	}
	c := newBrowser(b)
	w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword}, false)
	if w.Code != 403 {
		t.Fatalf("missing csrf %d", w.Code)
	}
	c.login(t, "admin", testPassword)
	w = c.request("GET", "/api/v1/users", nil, false)
	if w.Code != 401 {
		t.Fatal("forced-password user obtained admin access")
	}
	w = c.request("PUT", "/api/v1/me", map[string]string{"theme": "dark"}, true)
	if w.Code != 401 {
		t.Fatal("forced-password user changed profile")
	}
	w = c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "changed"}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	c.verifyMFA(t)
	w = c.request("GET", "/api/v1/users", nil, false)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = c.request("PUT", "/api/v1/me", map[string]string{"theme": "dark"}, true)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
func TestAdminSafetyAndRBAC(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			for _, body := range []map[string]any{{"enabled": false}, {"roleIds": []string{}}} {
				w := c.request("PUT", "/api/v1/users/"+admin.ID, body, true)
				if w.Code != 409 || !strings.Contains(w.Body.String(), "last_admin") {
					t.Fatalf("last admin: %d %s", w.Code, w.Body.String())
				}
			}
			w := c.request("DELETE", "/api/v1/users/"+admin.ID, nil, true)
			if w.Code != 409 {
				t.Fatalf("delete last admin %d %s", w.Code, w.Body.String())
			}
			u := User{ID: random(18), Username: "ordinary", Name: "ordinary", Enabled: true}
			u.PasswordHash, _ = passwordHash(testPassword)
			if e := b.DB.Create(&u).Error; e != nil {
				t.Fatal(e)
			}
			role := Role{ID: random(18), Name: "user manager"}
			b.DB.Create(&role)
			b.DB.Create(&RolePermission{RoleID: role.ID, PermissionID: "users:write"})
			b.DB.Create(&UserRole{UserID: u.ID, RoleID: role.ID})
			other := newBrowser(b)
			other.login(t, u.Username, testPassword)
			w = other.request("GET", "/api/v1/applications", nil, false)
			if w.Code != 403 {
				t.Fatal("ordinary user accessed applications")
			}
			for _, endpoint := range []string{"/api/v1/users/" + admin.ID + "/password", "/api/v1/users/" + admin.ID} {
				w = other.request("PUT", endpoint, map[string]any{"password": testPassword + "hacked", "enabled": false}, true)
				if w.Code != 403 && !(w.Code == 400 && strings.Contains(w.Body.String(), "forbidden")) {
					t.Fatalf("non-admin modified admin: %d %s", w.Code, w.Body.String())
				}
			}
			group := Group{ID: random(18), Name: "Readers"}
			b.DB.Create(&group)
			reader := Role{ID: random(18), Name: "reader"}
			b.DB.Create(&reader)
			b.DB.Create(&RolePermission{RoleID: reader.ID, PermissionID: "applications:read"})
			b.DB.Create(&GroupRole{GroupID: group.ID, RoleID: reader.ID})
			b.DB.Create(&GroupMember{GroupID: group.ID, UserID: u.ID})
			p, _, e := permissions(b.DB, u)
			if e != nil || !contains(p, "users:write") || !contains(p, "applications:read") {
				t.Fatalf("role union %v %v", p, e)
			}
		})
	}
}
func testApp(t *testing.T, b *Server, kind string) Application {
	t.Helper()
	app := Application{ID: random(18), ClientID: random(18), Name: "Test app", Enabled: true, ClientType: kind, RedirectURLs: []string{"http://client.example/callback"}, LogoutURLs: []string{"http://client.example/logout"}, Origins: []string{"http://client.example"}, LoginURL: "http://client.example/login"}
	if kind == "web" {
		app.SecretHash = hash("secret")
	}
	if e := b.DB.Create(&app).Error; e != nil {
		t.Fatal(e)
	}
	if e := b.DB.Create(&Permission{ID: "app:" + app.ID + ":login", Name: "app:" + app.ID + ":login", ApplicationID: app.ID}).Error; e != nil {
		t.Fatal(e)
	}
	return app
}
func authorize(t *testing.T, c *browser, a Application, extra url.Values) (string, string) {
	t.Helper()
	verifier := oauth2.GenerateVerifier()
	q := url.Values{"client_id": {a.ClientID}, "redirect_uri": {a.RedirectURLs[0]}, "response_type": {"code"}, "scope": {"openid profile email"}, "state": {"state-check"}, "nonce": {"nonce-check"}, "code_challenge": {oauth2.S256ChallengeFromVerifier(verifier)}, "code_challenge_method": {"S256"}}
	for k, v := range extra {
		q[k] = v
	}
	path := "/oidc/authorize?" + q.Encode()
	for i := 0; i < 5; i++ {
		w := c.request("GET", path, nil, false)
		if w.Code != 302 {
			t.Fatalf("authorize %s %d %s", path, w.Code, w.Body.String())
		}
		loc := w.Header().Get("Location")
		if strings.HasPrefix(loc, a.RedirectURLs[0]) {
			u, _ := url.Parse(loc)
			if u.Query().Get("error") != "" {
				return "error:" + u.Query().Get("error"), verifier
			}
			if u.Query().Get("state") != "state-check" {
				t.Fatal("state mismatch")
			}
			return u.Query().Get("code"), verifier
		}
		if strings.HasPrefix(loc, "/login") || strings.HasPrefix(loc, "/change-password") {
			return loc, verifier
		}
		path = loc
	}
	t.Fatal("redirect loop")
	return "", ""
}
func exchange(c *browser, a Application, code, verifier string) *httptest.ResponseRecorder {
	values := url.Values{"grant_type": {"authorization_code"}, "client_id": {a.ClientID}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {a.RedirectURLs[0]}}
	if a.ClientType == "spa" {
		return c.request("POST", "/oidc/token", values, false)
	}
	r := httptest.NewRequest("POST", "/oidc/token", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetBasicAuth(a.ClientID, "secret")
	w := httptest.NewRecorder()
	c.b.ServeHTTP(w, r)
	return w
}
func TestOIDCCodePKCEAndRevocation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, _ := testServer(t, driver)
			for _, kind := range []string{"spa", "web"} {
				t.Run(kind, func(t *testing.T) {
					a := testApp(t, b, kind)
					code, v := authorize(t, c, a, nil)
					wrong := exchange(c, a, code, oauth2.GenerateVerifier())
					if wrong.Code == 200 {
						t.Fatal("invalid PKCE accepted")
					}
					w := exchange(c, a, code, v)
					if w.Code != 200 {
						t.Fatalf("exchange %d %s", w.Code, w.Body.String())
					}
					var tokens map[string]any
					json.Unmarshal(w.Body.Bytes(), &tokens)
					if _, exists := tokens["refresh_token"]; exists {
						t.Fatal("refresh token emitted")
					}
					raw, _ := tokens["id_token"].(string)
					signed, e := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
					if e != nil {
						t.Fatal(e)
					}
					var key SigningKey
					b.DB.Where("active = ?", true).First(&key)
					var jwk jose.JSONWebKey
					json.Unmarshal([]byte(key.PublicJSON), &jwk)
					payload, e := signed.Verify(jwk.Key)
					if e != nil {
						t.Fatal(e)
					}
					var claims map[string]any
					json.Unmarshal(payload, &claims)
					amr, ok := claims["amr"].([]any)
					if !ok || len(amr) != 2 || amr[0] != "pwd" || amr[1] != "otp" || claims["auth_time"] == nil {
						t.Fatal("ID Token does not describe completed password and OTP authentication")
					}
					if claims["iss"] != b.Config.Issuer || claims["nonce"] != "nonce-check" {
						t.Fatalf("claims %s", payload)
					}
					if exchange(c, a, code, v).Code == 200 {
						t.Fatal("code replay accepted")
					}
					req := httptest.NewRequest("GET", "/oidc/userinfo", nil)
					req.Header.Set("Authorization", "Bearer "+tokens["access_token"].(string))
					out := httptest.NewRecorder()
					b.ServeHTTP(out, req)
					if out.Code != 200 {
						t.Fatalf("userinfo %d %s", out.Code, out.Body.String())
					}
					if e := b.RotateKeys(); e != nil {
						t.Fatal(e)
					}
					if _, e = signed.Verify(jwk.Key); e != nil {
						t.Fatal("old signing key no longer verifies")
					}
					b.DB.Model(&Session{}).Where("credential_hash = ?", hash(c.cookies["burrow_session"].Value)).Update("revoked", true)
					out = httptest.NewRecorder()
					b.ServeHTTP(out, req)
					if out.Code == 200 {
						t.Fatal("userinfo accepted revoked session")
					}
					c.login(t, "admin", testPassword+"-changed")
				})
			}
		})
	}
}
func TestConcurrentCodeExchange(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, _ := testServer(t, driver)
			a := testApp(t, b, "spa")
			code, v := authorize(t, c, a, nil)
			var wg sync.WaitGroup
			results := make(chan int, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); results <- exchange(newBrowser(b), a, code, v).Code }()
			}
			wg.Wait()
			close(results)
			success := 0
			for code := range results {
				if code == 200 {
					success++
				}
			}
			if success != 1 {
				t.Fatalf("concurrent successful exchanges = %d", success)
			}
		})
	}
}
func TestOIDCRejectsStaleAuthorizationAndPrompt(t *testing.T) {
	b, c, admin := testServer(t, "sqlite")
	a := testApp(t, b, "spa")
	code, v := authorize(t, c, a, nil)
	b.DB.Model(&Application{}).Where("id = ?", a.ID).Update("enabled", false)
	if exchange(c, a, code, v).Code == 200 {
		t.Fatal("disabled app exchanged code")
	}
	b.DB.Model(&Application{}).Where("id = ?", a.ID).Update("enabled", true)
	code, v = authorize(t, c, a, nil)
	b.DB.Model(&User{}).Where("id = ?", admin.ID).Update("enabled", false)
	if exchange(c, a, code, v).Code == 200 {
		t.Fatal("disabled user exchanged code")
	}
	b.DB.Model(&User{}).Where("id = ?", admin.ID).Update("enabled", true)
	loggedOut := newBrowser(b)
	code, _ = authorize(t, loggedOut, a, url.Values{"prompt": {"none"}})
	if code != "error:login_required" {
		t.Fatalf("prompt none %s", code)
	}
	code, _ = authorize(t, c, a, url.Values{"prompt": {"login"}})
	if !strings.HasPrefix(code, "/login") {
		t.Fatalf("prompt login reused session: %s", code)
	}
	w := c.request("GET", "/oidc/authorize?client_id="+a.ClientID+"&redirect_uri=https://evil.example", nil, false)
	if w.Code != 400 || w.Header().Get("Location") != "" {
		t.Fatal("untrusted redirect accepted")
	}
	var session Session
	b.DB.Where("credential_hash = ?", hash(c.cookies["burrow_session"].Value)).First(&session)
	b.DB.Model(&session).Update("expires_at", time.Now().Add(-time.Minute))
	code, _ = authorize(t, c, a, url.Values{"prompt": {"none"}})
	if code != "error:login_required" {
		t.Fatal("expired session accepted")
	}
}
func TestPasswordAndEncryption(t *testing.T) {
	s := testStore(t, "sqlite")
	h, e := passwordHash(testPassword)
	if e != nil || !passwordOK(h, testPassword) || passwordOK(h, "wrong") {
		t.Fatal("password hash verification")
	}
	v, e := s.seal("secret")
	if e != nil {
		t.Fatal(e)
	}
	s.Config.MasterKey[0] ^= 1
	if _, e = s.unseal(v); e == nil {
		t.Fatal("wrong encryption key accepted")
	}
}
