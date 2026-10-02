package burrow

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// Build historical schemas from the original SQL rather than downgrading a
// current schema whose removed columns and tables no longer exist.
func testLegacyStore(t *testing.T, driver string, version int) *Store {
	t.Helper()
	s := openTestStore(t, driver)
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Migrator().CreateTable(&SchemaVersion{}); err != nil {
			return err
		}
		for _, migration := range []string{initialMigration, pkceCompatibilityMigration, customRolesMigration, removeUpstreamMigration}[:version] {
			for _, statement := range strings.Split(migration, ";") {
				if strings.TrimSpace(statement) != "" {
					if err := tx.Exec(statement).Error; err != nil {
						return err
					}
				}
			}
		}
		return tx.Create(&SchemaVersion{ID: 1, Version: version, Checksum: migrationChecksum(version)}).Error
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func createLegacyUser(t *testing.T, s *Store, u User, local bool) {
	t.Helper()
	if err := s.DB.Table("users").Create(map[string]any{
		"id": u.ID, "username": u.Username, "name": u.Name, "email": u.Email,
		"enabled": u.Enabled, "local_enabled": local, "password_hash": u.PasswordHash,
		"must_change_password": u.MustChangePassword, "language": u.Language, "theme": u.Theme,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func createLegacyApplication(t *testing.T, s *Store, a Application, local bool) {
	t.Helper()
	redirects, _ := json.Marshal(a.RedirectURLs)
	logouts, _ := json.Marshal(a.LogoutURLs)
	origins, _ := json.Marshal(a.Origins)
	values := map[string]any{
		"id": a.ID, "name": a.Name, "client_id": a.ClientID, "client_type": a.ClientType,
		"secret_hash": a.SecretHash, "enabled": a.Enabled, "local_enabled": local,
		"icon": a.Icon, "login_url": a.LoginURL, "redirect_urls": string(redirects),
		"logout_urls": string(logouts), "origins": string(origins),
	}
	if s.DB.Migrator().HasColumn("applications", "allow_without_pkce") {
		values["allow_without_pkce"] = a.AllowWithoutPKCE
	}
	if err := s.DB.Table("applications").Create(values).Error; err != nil {
		t.Fatal(err)
	}
}

func TestRemoveUpstreamMigration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, scenario := range []string{"preserve", "upstream-user", "missing-password", "upstream-app", "audit-rollback"} {
			t.Run(driver+"/"+scenario, func(t *testing.T) {
				s := testLegacyStore(t, driver, 3)
				h, err := passwordHash(testPassword)
				if err != nil {
					t.Fatal(err)
				}
				u := User{ID: "local-user", Username: "local", Name: "Local User", Email: "local@example.test", Enabled: true, PasswordHash: h, Language: "zh-CN", Theme: "dark"}
				if scenario == "missing-password" {
					u.PasswordHash = ""
				}
				createLegacyUser(t, s, u, scenario != "upstream-user")
				disabled := User{ID: "disabled-user", Username: "disabled", Name: "Disabled upstream user", Enabled: false}
				createLegacyUser(t, s, disabled, false)
				a := Application{ID: "local-app", ClientID: "local-client", Name: "Local App", ClientType: "web", SecretHash: hash("existing-secret"), Enabled: true, AllowWithoutPKCE: true, RedirectURLs: []string{"http://client.example/callback"}, LoginURL: "http://client.example/login"}
				createLegacyApplication(t, s, a, scenario != "upstream-app")
				createLegacyApplication(t, s, Application{ID: "disabled-app", ClientID: "disabled-client", Enabled: false}, false)
				if err := s.RotateKeys(); err != nil {
					t.Fatal(err)
				}
				var originalKey SigningKey
				if err := s.DB.First(&originalKey).Error; err != nil {
					t.Fatal(err)
				}
				for _, statement := range []string{
					"INSERT INTO roles (id, name, builtin) VALUES ('team', 'Team', FALSE)",
					"INSERT INTO groups (id, name) VALUES ('team-group', 'Team Group')",
					"INSERT INTO user_roles (user_id, role_id) VALUES ('local-user', 'team')",
					"INSERT INTO group_members (user_id, group_id) VALUES ('local-user', 'team-group')",
					"INSERT INTO group_roles (group_id, role_id) VALUES ('team-group', 'team')",
					"INSERT INTO permissions (id, name, application_id) VALUES ('app:local-app:login', 'app:local-app:login', 'local-app')",
					"INSERT INTO role_permissions (role_id, permission_id) VALUES ('team', 'providers:read'), ('team', 'providers:write'), ('team', 'applications:read'), ('team', 'app:local-app:login')",
					"INSERT INTO providers (id, name, issuer, client_id, secret_cipher, enabled) VALUES ('upstream', 'Upstream', 'https://issuer.example', 'rp', 'old-ciphertext', TRUE)",
					"INSERT INTO external_identities (id, user_id, provider_id, issuer, subject) VALUES ('identity', 'disabled-user', 'upstream', 'https://issuer.example', 'external-subject')",
					"INSERT INTO application_providers (application_id, provider_id) VALUES ('local-app', 'upstream')",
					"INSERT INTO upstream_transactions (id, provider_id, consumed) VALUES ('upstream-login', 'upstream', FALSE)",
				} {
					if err := s.DB.Exec(statement).Error; err != nil {
						t.Fatal(err)
					}
				}
				for _, method := range []string{"password", "oidc", "unknown"} {
					session := Session{ID: method, UserID: u.ID, CredentialHash: hash(method), Method: method, AuthTime: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
					if err := s.DB.Omit("MFAAt", "AuthVersion").Create(&session).Error; err != nil {
						t.Fatal(err)
					}
					if err := s.DB.Create(&TokenRecord{ID: method, UserID: u.ID, ClientID: a.ID, SessionID: method, ExpiresAt: session.ExpiresAt}).Error; err != nil {
						t.Fatal(err)
					}
					if err := s.DB.Create(&AuthTransaction{ID: method, ClientID: a.ID, UserID: u.ID, SessionID: method, Method: method, ExpiresAt: session.ExpiresAt}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "audit-rollback" {
					if err := s.DB.Migrator().DropTable(&Event{}); err != nil {
						t.Fatal(err)
					}
				}
				if s.Health(context.Background()) == nil {
					t.Fatal("legacy schema was ready before migration")
				}
				err = s.Migrate()
				if scenario != "preserve" {
					if err == nil {
						t.Fatal("unsafe or unaudited upgrade accepted")
					}
					if scenario != "audit-rollback" && !strings.Contains(err.Error(), "requires password login") {
						t.Fatal(err)
					}
					var version SchemaVersion
					if err := s.DB.First(&version, 1).Error; err != nil || version.Version != 3 || version.Checksum != migrationChecksum(3) {
						t.Fatal("failed migration advanced schema")
					}
					if !s.DB.Migrator().HasTable("providers") || !s.DB.Migrator().HasColumn("users", "local_enabled") {
						t.Fatal("failed migration removed legacy data")
					}
					var session Session
					if err := s.DB.First(&session, "id = ?", "oidc").Error; err != nil || session.Revoked {
						t.Fatal("failed migration revoked session")
					}
					var grants int64
					if err := s.DB.Model(&RolePermission{}).Where("role_id = ?", "team").Count(&grants).Error; err != nil || grants != 4 {
						t.Fatal("failed migration changed authorization")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Migrate(); err != nil {
					t.Fatal(err)
				}
				if err := s.Health(context.Background()); err != nil {
					t.Fatal(err)
				}
				for _, table := range []string{"providers", "external_identities", "application_providers", "upstream_transactions"} {
					if s.DB.Migrator().HasTable(table) {
						t.Fatalf("legacy table remains: %s", table)
					}
				}
				for table, column := range map[string]string{"users": "local_enabled", "applications": "local_enabled", "sessions": "provider_id"} {
					if s.DB.Migrator().HasColumn(table, column) {
						t.Fatalf("legacy column remains: %s.%s", table, column)
					}
				}
				var current User
				u.MFALastStep = -1
				if err := s.DB.First(&current, "id = ?", u.ID).Error; err != nil || !reflect.DeepEqual(current, u) {
					t.Fatal("user data changed during migration")
				}
				current = User{}
				if err := s.DB.First(&current, "id = ?", disabled.ID).Error; err != nil || current.Enabled || current.PasswordHash != "" {
					t.Fatal("disabled upstream account was enabled or lost")
				}
				var currentApp Application
				if err := s.DB.First(&currentApp, "id = ?", a.ID).Error; err != nil || currentApp.SecretHash != a.SecretHash || !currentApp.AllowWithoutPKCE || !currentApp.Enabled || currentApp.Name != a.Name || currentApp.RedirectURLs[0] != a.RedirectURLs[0] {
					t.Fatal("application data changed during migration")
				}
				currentApp = Application{}
				if err := s.DB.First(&currentApp, "id = ?", "disabled-app").Error; err != nil || currentApp.Enabled {
					t.Fatal("disabled application was enabled or lost")
				}
				var currentKey SigningKey
				if err := s.DB.First(&currentKey).Error; err != nil || currentKey != originalKey {
					t.Fatal("migration replaced signing keys")
				}
				p, _, err := permissions(s.DB, u)
				if err != nil || len(p) != 2 || !contains(p, "applications:read") || !contains(p, "app:local-app:login") {
					t.Fatalf("non-Provider grants changed: %v %v", p, err)
				}
				for table, expected := range map[string]int64{"user_roles": 1, "group_members": 1, "group_roles": 1} {
					var count int64
					if err := s.DB.Table(table).Count(&count).Error; err != nil || count != expected {
						t.Fatalf("membership changed: %s", table)
					}
				}
				for _, method := range []string{"password", "oidc", "unknown"} {
					var session Session
					var token TokenRecord
					var auths int64
					if err := s.DB.First(&session, "id = ?", method).Error; err != nil {
						t.Fatal(err)
					}
					if err := s.DB.First(&token, "id = ?", method).Error; err != nil {
						t.Fatal(err)
					}
					if err := s.DB.Model(&AuthTransaction{}).Where("id = ?", method).Count(&auths).Error; err != nil {
						t.Fatal(err)
					}
					if !session.Revoked || !token.Revoked || auths != 0 {
						t.Fatal("non-password authentication remains usable")
					}
				}
				var count int64
				if err := s.DB.Model(&Event{}).Where("id = ?", "migration:004:remove_upstream").Count(&count).Error; err != nil || count != 1 {
					t.Fatal("migration audit absent or duplicated")
				}
			})
		}
	}
}

func TestPasswordOnlyAuthentication(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			for _, endpoint := range []string{"/api/v1/providers", "/api/v1/providers/legacy", "/api/v1/auth/providers", "/api/v1/auth/providers/legacy/login", "/api/v1/auth/providers/legacy/callback", "/api/v1/users/" + admin.ID + "/identities", "/api/v1/users/" + admin.ID + "/identities/legacy"} {
				for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
					if w := c.request(method, endpoint, nil, true); w.Code != 404 {
						t.Fatalf("removed endpoint reachable: %s %s: %d", method, endpoint, w.Code)
					}
				}
			}
			if w := c.request("POST", "/api/v1/users", map[string]any{"username": "without-password"}, true); w.Code != 400 {
				t.Fatal("new user created without password")
			}
			a := testApp(t, b, "spa")
			for _, update := range []struct {
				path, field string
				value       any
			}{
				{"/api/v1/users/" + admin.ID, "localEnabled", false},
				{"/api/v1/applications/" + a.ID, "localEnabled", false},
				{"/api/v1/applications/" + a.ID, "providerIds", []string{}},
			} {
				if w := c.request("PUT", update.path, map[string]any{update.field: update.value}, true); w.Code != 400 {
					t.Fatalf("removed field accepted: %s", update.field)
				}
			}
			// Disabled historical users without a password remain manageable. They
			// must receive a temporary password before they can be enabled.
			u := User{ID: random(18), Username: "disabled-legacy", Language: "en", Theme: "system"}
			if err := b.DB.Create(&u).Error; err != nil {
				t.Fatal(err)
			}
			if w := c.request("PUT", "/api/v1/users/"+u.ID, map[string]any{"name": "updated"}, true); w.Code != 200 {
				t.Fatal("disabled legacy user cannot be edited")
			}
			if w := c.request("PUT", "/api/v1/users/"+u.ID, map[string]any{"enabled": true}, true); w.Code != 400 {
				t.Fatal("legacy user enabled without password")
			}
			if w := c.request("PUT", "/api/v1/users/"+u.ID+"/password", map[string]string{"password": testPassword}, true); w.Code != 200 {
				t.Fatal("legacy user password cannot be reset")
			}
			guest := newBrowser(b)
			guest.init(t)
			if w := guest.request("POST", "/api/v1/auth/login", map[string]string{"username": u.Username, "password": testPassword}, true); w.Code != 401 {
				t.Fatal("password reset enabled disabled account")
			}
			if w := c.request("PUT", "/api/v1/users/"+u.ID, map[string]any{"enabled": true}, true); w.Code != 200 {
				t.Fatal("legacy user cannot be enabled after reset")
			}
			guest.login(t, u.Username, testPassword)
			var recovered User
			if err := b.DB.First(&recovered, "id = ?", u.ID).Error; err != nil || !recovered.MustChangePassword {
				t.Fatal("password reset did not require password change")
			}
			if w := guest.request("PUT", "/api/v1/me", map[string]string{"name": "bypass"}, true); w.Code != 401 {
				t.Fatal("legacy user bypassed forced password change")
			}
			if w := guest.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-changed"}, true); w.Code != 200 {
				t.Fatal("legacy user cannot complete password change")
			}
			guest.verifyMFA(t)
			if w := guest.request("GET", "/api/v1/me/apps", nil, false); w.Code != 200 {
				t.Fatal("legacy user cannot access portal after recovery")
			}
			// Defense in depth: an old or unrecognized method cannot access the
			// API, complete an authorization, exchange a code, or call UserInfo.
			for _, method := range []string{"oidc", "unknown", ""} {
				tokenCode, tokenVerifier := authorize(t, c, a, nil)
				response := exchange(c, a, tokenCode, tokenVerifier)
				var tokens struct {
					AccessToken string `json:"access_token"`
				}
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &tokens) != nil || tokens.AccessToken == "" {
					t.Fatal("failed to issue password-session token")
				}
				code, verifier := authorize(t, c, a, nil)
				if err := b.DB.Model(&Session{}).Where("credential_hash = ?", hash(c.cookies["burrow_session"].Value)).Update("method", method).Error; err != nil {
					t.Fatal(err)
				}
				if w := c.request("GET", "/api/v1/me", nil, false); w.Code != 401 {
					t.Fatal("non-password session authenticated")
				}
				if w := exchange(c, a, code, verifier); w.Code == 200 {
					t.Fatal("non-password session exchanged authorization code")
				}
				if result, _ := authorize(t, c, a, url.Values{"prompt": {"none"}}); result != "error:login_required" {
					t.Fatal("non-password session authorized")
				}
				r := httptest.NewRequest("GET", "/oidc/userinfo", nil)
				r.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
				w := httptest.NewRecorder()
				b.ServeHTTP(w, r)
				if w.Code == 200 {
					t.Fatal("non-password session called UserInfo")
				}
				c.login(t, admin.Username, testPassword+"-changed")
			}
		})
	}
}
