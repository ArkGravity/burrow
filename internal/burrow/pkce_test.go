package burrow

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"
)

func withoutPKCE() url.Values {
	return url.Values{"code_challenge": {""}, "code_challenge_method": {""}}
}

func TestPKCECompatibilityProtocol(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			web := testApp(t, b, "web")
			spa := testApp(t, b, "spa")
			for _, app := range []Application{web, spa} {
				code, _ := authorize(t, c, app, withoutPKCE())
				if code != "error:invalid_request" {
					t.Fatalf("default %s accepted missing PKCE: %s", app.ClientType, code)
				}
			}
			if w := c.request("PUT", "/api/v1/applications/"+web.ID, map[string]any{"allowWithoutPkce": true}, true); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			for _, extra := range []url.Values{
				{"code_challenge_method": {"plain"}},
				{"code_challenge_method": {""}},
				{"code_challenge": {""}},
				{"code_challenge": {strings.Repeat("!", 43)}},
				{"code_challenge": {strings.Repeat("A", 42) + "B"}},
			} {
				code, _ := authorize(t, c, web, extra)
				if code != "error:invalid_request" {
					t.Fatalf("malformed PKCE accepted: %v => %s", extra, code)
				}
			}
			code, verifier := authorize(t, c, web, nil)
			for _, invalid := range []string{"", oauth2.GenerateVerifier()} {
				if exchange(c, web, code, invalid).Code == 200 {
					t.Fatal("compatibility mode bypassed submitted PKCE")
				}
			}
			if w := exchange(c, web, code, verifier); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			noPKCE := withoutPKCE()
			noPKCE["nonce"] = []string{""} // Match clients such as Nightingale v9.1.1.
			code, _ = authorize(t, c, web, noPKCE)
			values := url.Values{"grant_type": {"authorization_code"}, "client_id": {web.ClientID}, "code": {code}, "redirect_uri": {web.RedirectURLs[0]}}
			for _, secret := range []string{"", "wrong"} {
				r := httptest.NewRequest("POST", "/oidc/token", strings.NewReader(values.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if secret != "" {
					r.SetBasicAuth(web.ClientID, secret)
				}
				w := httptest.NewRecorder()
				b.ServeHTTP(w, r)
				if w.Code == 200 {
					t.Fatal("missing or wrong client secret accepted")
				}
			}
			if exchange(c, web, code, oauth2.GenerateVerifier()).Code == 200 {
				t.Fatal("unexpected verifier accepted without challenge")
			}
			w := exchange(c, web, code, "")
			if w.Code != 200 {
				t.Fatalf("no-PKCE exchange: %d %s", w.Code, w.Body.String())
			}
			var tokens map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil {
				t.Fatal(err)
			}
			if tokens["id_token"] == nil || tokens["access_token"] == nil || tokens["refresh_token"] != nil {
				t.Fatal("unexpected token response")
			}
			if exchange(c, web, code, "").Code == 200 {
				t.Fatal("no-PKCE code replay accepted")
			}
			code, _ = authorize(t, c, web, withoutPKCE())
			var wg sync.WaitGroup
			results := make(chan int, 4)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); results <- exchange(newBrowser(b), web, code, "").Code }()
			}
			wg.Wait()
			close(results)
			success := 0
			for status := range results {
				if status == 200 {
					success++
				}
			}
			if success != 1 {
				t.Fatalf("concurrent no-PKCE exchanges = %d", success)
			}
			for _, change := range []string{"policy", "user", "application", "session"} {
				code, _ = authorize(t, c, web, withoutPKCE())
				switch change {
				case "policy":
					if w := c.request("PUT", "/api/v1/applications/"+web.ID, map[string]any{"allowWithoutPkce": false}, true); w.Code != 200 {
						t.Fatal(w.Body.String())
					}
					r := httptest.NewRequest("GET", "/oidc/userinfo", nil)
					r.Header.Set("Authorization", "Bearer "+tokens["access_token"].(string))
					out := httptest.NewRecorder()
					b.ServeHTTP(out, r)
					if out.Code != 200 {
						t.Fatalf("policy change invalidated issued token: %d %s", out.Code, out.Body.String())
					}
				case "user":
					b.DB.Model(&User{}).Where("id = ?", admin.ID).Update("enabled", false)
				case "application":
					b.DB.Model(&Application{}).Where("id = ?", web.ID).Update("enabled", false)
				case "session":
					b.DB.Model(&Session{}).Where("credential_hash = ?", hash(c.cookies["burrow_session"].Value)).Update("revoked", true)
				}
				if exchange(c, web, code, "").Code == 200 {
					t.Fatalf("stale %s accepted", change)
				}
				b.DB.Model(&Application{}).Where("id = ?", web.ID).Updates(map[string]any{"allow_without_pkce": true, "enabled": true})
				b.DB.Model(&User{}).Where("id = ?", admin.ID).Update("enabled", true)
				if change == "session" {
					c.login(t, "admin", testPassword+"-changed")
				}
			}
			// Even corrupted persisted SPA settings must not relax PKCE.
			b.DB.Model(&Application{}).Where("id = ?", spa.ID).Update("allow_without_pkce", true)
			if code, _ := authorize(t, c, spa, withoutPKCE()); code != "error:invalid_request" {
				t.Fatal("SPA bypassed PKCE")
			}
		})
	}
}

func TestPKCECompatibilityAdministration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, admin := testServer(t, driver)
			var session map[string]any
			if w := c.request("GET", "/api/v1/me", nil, false); json.Unmarshal(w.Body.Bytes(), &session) != nil || session["administrator"] != true {
				t.Fatal("administrator session not identified")
			}
			app := testApp(t, b, "web")
			spa := testApp(t, b, "spa")
			for _, bad := range []any{true, nil, "true"} {
				if w := c.request("PUT", "/api/v1/applications/"+spa.ID, map[string]any{"allowWithoutPkce": bad}, true); w.Code != 400 {
					t.Fatalf("invalid SPA setting: %d", w.Code)
				}
			}
			if err := b.DB.Create(&Role{ID: "editor", Name: "Application maintainer"}).Error; err != nil {
				t.Fatal(err)
			}
			for _, permission := range []string{"applications:read", "applications:write"} {
				if err := b.DB.Create(&RolePermission{RoleID: "editor", PermissionID: permission}).Error; err != nil {
					t.Fatal(err)
				}
			}
			h, err := passwordHash(testPassword)
			if err != nil {
				t.Fatal(err)
			}
			u := User{ID: random(18), Username: "editor", PasswordHash: h, Enabled: true}
			if err := b.DB.Create(&u).Error; err != nil {
				t.Fatal(err)
			}
			if err := b.DB.Create(&UserRole{UserID: u.ID, RoleID: "editor"}).Error; err != nil {
				t.Fatal(err)
			}
			editor := newBrowser(b)
			editor.login(t, u.Username, testPassword)
			if w := editor.request("GET", "/api/v1/me", nil, false); json.Unmarshal(w.Body.Bytes(), &session) != nil || session["administrator"] != false {
				t.Fatal("editor session identified as administrator")
			}
			if w := editor.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"allowWithoutPkce": true}, true); w.Code != 403 {
				t.Fatalf("editor enabled exception: %d", w.Code)
			}
			body := map[string]any{"name": "created compatibility app", "redirectUris": app.RedirectURLs, "loginUrl": app.LoginURL, "allowWithoutPkce": true}
			if w := editor.request("POST", "/api/v1/applications", body, true); w.Code != 403 {
				t.Fatalf("editor created exception: %d", w.Code)
			}
			if w := c.request("POST", "/api/v1/applications", body, true); w.Code != 201 {
				t.Fatal(w.Body.String())
			}
			delete(body, "allowWithoutPkce")
			w := editor.request("POST", "/api/v1/applications", body, true)
			var created Application
			if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &created) != nil || created.AllowWithoutPKCE {
				t.Fatal("default application relaxed PKCE")
			}
			for _, enabled := range []bool{true, false} {
				if w := c.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"allowWithoutPkce": enabled}, true); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				var event Event
				if err := b.DB.Where("object_id = ? AND kind = ?", app.ID, "applications:put").Order("created_at DESC").First(&event).Error; err != nil {
					t.Fatal(err)
				}
				var details map[string]map[string]bool
				if json.Unmarshal([]byte(event.Details), &details) != nil || details["allowWithoutPkce"]["before"] != !enabled || details["allowWithoutPkce"]["after"] != enabled {
					t.Fatalf("missing policy audit: %s", event.Details)
				}
				if w := editor.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"allowWithoutPkce": !enabled}, true); w.Code != 403 {
					t.Fatal("editor changed policy")
				}
				if w := editor.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"name": "editor update"}, true); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			}
			w = interleave(c, "PUT", "/api/v1/applications/"+app.ID, `{"allowWithoutPkce":true}`, func() {
				if err := b.DB.Where("user_id = ?", admin.ID).Delete(&UserRole{}).Error; err != nil {
					t.Fatal(err)
				}
				if err := b.DB.Create(&UserRole{UserID: admin.ID, RoleID: "editor"}).Error; err != nil {
					t.Fatal(err)
				}
			})
			if w.Code != 403 {
				t.Fatal("stale administrator permission accepted")
			}
			b.DB.Where("user_id = ?", admin.ID).Delete(&UserRole{})
			b.DB.Create(&UserRole{UserID: admin.ID, RoleID: "admin"})
			if err := b.DB.Migrator().DropTable(&Event{}); err != nil {
				t.Fatal(err)
			}
			w = c.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"allowWithoutPkce": true}, true)
			var current Application
			if err := b.DB.First(&current, "id = ?", app.ID).Error; err != nil {
				t.Fatal(err)
			}
			if w.Code == 200 || current.AllowWithoutPKCE {
				t.Fatal("policy change committed without audit")
			}
		})
	}
}

func TestPKCEMigrationUpgrade(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := testLegacyStore(t, driver, 1)
			app := Application{ID: random(18), ClientID: random(18), ClientType: "web", Enabled: true, Name: "existing app", SecretHash: "existing hash", AllowWithoutPKCE: true}
			createLegacyApplication(t, s, app, true)
			if err := s.DB.Model(&SchemaVersion{}).Where("id = 1").Updates(map[string]any{"version": 1, "checksum": hash(initialMigration)}).Error; err != nil {
				t.Fatal(err)
			}
			if s.Health(context.Background()) == nil {
				t.Fatal("old schema considered ready")
			}
			if err := s.DB.Model(&SchemaVersion{}).Where("id = 1").Update("checksum", "tampered").Error; err != nil {
				t.Fatal(err)
			}
			if s.Migrate() == nil {
				t.Fatal("tampered version-one migration accepted")
			}
			if err := s.DB.Model(&SchemaVersion{}).Where("id = 1").Update("checksum", hash(initialMigration)).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.Migrate(); err != nil {
				t.Fatal(err)
			}
			if err := s.Migrate(); err != nil {
				t.Fatal(err)
			}
			if err := s.Health(context.Background()); err != nil {
				t.Fatal(err)
			}
			var current Application
			if err := s.DB.First(&current, "id = ?", app.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.AllowWithoutPKCE || current.SecretHash != app.SecretHash || current.Name != app.Name {
				t.Fatal("upgrade changed application or relaxed PKCE")
			}
		})
	}
}
