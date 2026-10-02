package burrow

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestApplicationCredentials(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, _ := testServer(t, driver)
			payload := func() map[string]any {
				return map[string]any{"name": "Grafana", "clientType": "web", "loginUrl": "http://client.example/login", "redirectUris": []string{"http://client.example/callback"}}
			}
			input := payload()
			input["clientId"] = "grafana-example"
			input["clientSecret"] = "grafana-example-secret"
			w := c.request("POST", "/api/v1/applications", input, true)
			if w.Code != 201 {
				t.Fatal(w.Code, w.Body.String())
			}
			var created struct {
				Application
				Secret string `json:"clientSecret"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if created.ClientID != "grafana-example" || created.Secret != "grafana-example-secret" {
				t.Fatal("custom credentials not accepted")
			}
			var persisted Application
			if err := b.DB.First(&persisted, "id = ?", created.ID).Error; err != nil {
				t.Fatal(err)
			}
			if persisted.SecretHash != hash(created.Secret) {
				t.Fatal("secret not hashed")
			}
			for _, path := range []string{"/api/v1/applications"} {
				response := c.request("GET", path, nil, false)
				if response.Code != 200 || strings.Contains(response.Body.String(), created.Secret) || strings.Contains(response.Body.String(), persisted.SecretHash) {
					t.Fatal("credentials leaked in read/audit response")
				}
			}
			var events []Event
			if err := b.DB.Where("object_id = ?", created.ID).Find(&events).Error; err != nil {
				t.Fatal(err)
			}
			audit, _ := json.Marshal(events)
			if len(events) == 0 || strings.Contains(string(audit), created.Secret) || strings.Contains(string(audit), persisted.SecretHash) {
				t.Fatal("secret leaked in audit")
			}
			if duplicate := c.request("POST", "/api/v1/applications", input, true); duplicate.Code != 409 {
				t.Fatal("duplicate client ID not rejected", duplicate.Code)
			}
			code, verifier := authorize(t, c, persisted, nil)
			values := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {persisted.RedirectURLs[0]}, "code_verifier": {verifier}}
			for _, secret := range []string{"wrong-secret", created.Secret} {
				req := httptest.NewRequest("POST", "/oidc/token", strings.NewReader(values.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.SetBasicAuth(persisted.ClientID, secret)
				out := httptest.NewRecorder()
				b.ServeHTTP(out, req)
				if (out.Code == 200) != (secret == created.Secret) {
					t.Fatal("custom secret authentication failed", out.Code)
				}
			}
			for _, change := range []map[string]any{{"clientSecret": "replacement-secret"}, {"clientSecret": ""}, {"clientId": "changed-id"}} {
				if out := c.request("PUT", "/api/v1/applications/"+created.ID, change, true); out.Code != 400 {
					t.Fatal("credentials changed through update")
				}
			}
			for _, field := range []string{"clientId", "clientSecret"} {
				bad := []any{nil, 42, false, " ", "invalid:value", "nonascii中文", strings.Repeat("x", 257)}
				if field == "clientSecret" {
					bad = []any{nil, 42, false, "short", strings.Repeat("x", 16) + "\n", "spaces not accepted", strings.Repeat("x", 257)}
				}
				for _, value := range bad {
					invalid := payload()
					invalid[field] = value
					if out := c.request("POST", "/api/v1/applications", invalid, true); out.Code != 400 {
						t.Fatalf("invalid %s accepted: %v (%d)", field, value, out.Code)
					}
				}
			}
			for _, blank := range []bool{false, true} {
				automatic := payload()
				if blank {
					automatic["clientId"] = ""
					automatic["clientSecret"] = ""
				}
				out := c.request("POST", "/api/v1/applications", automatic, true)
				var result map[string]any
				if out.Code != 201 || json.Unmarshal(out.Body.Bytes(), &result) != nil || result["clientId"] == "" || result["clientSecret"] == "" || result["clientSecret"] == nil {
					t.Fatal("automatic credentials failed")
				}
			}
			spa := payload()
			spa["clientType"] = "spa"
			spa["clientSecret"] = created.Secret
			if out := c.request("POST", "/api/v1/applications", spa, true); out.Code != 400 {
				t.Fatal("SPA secret accepted")
			}
			delete(spa, "clientSecret")
			if out := c.request("POST", "/api/v1/applications", spa, true); out.Code != 201 || strings.Contains(out.Body.String(), "clientSecret") {
				t.Fatal("SPA creation failed")
			}
			anonymous := newBrowser(b)
			anonymous.init(t)
			if out := anonymous.request("POST", "/api/v1/applications", payload(), true); out.Code != 401 {
				t.Fatal("anonymous creation allowed")
			}
			reset := c.request("POST", "/api/v1/applications/"+created.ID+"/secret", map[string]any{}, true)
			var rotation map[string]string
			if reset.Code != 200 || json.Unmarshal(reset.Body.Bytes(), &rotation) != nil || rotation["clientSecret"] == created.Secret {
				t.Fatal("rotation failed")
			}
			b.DB.First(&persisted, "id = ?", created.ID)
			if persisted.SecretHash != hash(rotation["clientSecret"]) {
				t.Fatal("rotation not persisted")
			}
			h, err := passwordHash(testPassword)
			if err != nil {
				t.Fatal(err)
			}
			for _, role := range []string{"unassigned", "operator"} {
				if role == "operator" {
					b.DB.Create(&Role{ID: role, Name: "Application maintainer"})
					b.DB.Create(&RolePermission{RoleID: role, PermissionID: "applications:write"})
				}
				u := User{ID: random(18), Username: role, PasswordHash: h, Enabled: true}
				if err := b.DB.Create(&u).Error; err != nil {
					t.Fatal(err)
				}
				if role == "operator" {
					if err := b.DB.Create(&UserRole{UserID: u.ID, RoleID: role}).Error; err != nil {
						t.Fatal(err)
					}
				}
				actor := newBrowser(b)
				actor.login(t, role, testPassword)
				body := payload()
				body["clientId"] = role + "-application"
				body["clientSecret"] = "another-example-secret"
				out := actor.request("POST", "/api/v1/applications", body, true)
				want := 403
				if role == "operator" {
					want = 201
				}
				if out.Code != want {
					t.Fatalf("%s credential creation status %d", role, out.Code)
				}
			}
			if err := b.DB.Migrator().DropTable(&Event{}); err != nil {
				t.Fatal(err)
			}
			body := payload()
			body["clientId"] = "audit-rollback"
			body["clientSecret"] = "rollback-example-secret"
			if out := c.request("POST", "/api/v1/applications", body, true); out.Code != 503 {
				t.Fatal("audit failure accepted")
			}
			var count int64
			b.DB.Model(&Application{}).Where("client_id = ?", "audit-rollback").Count(&count)
			if count != 0 {
				t.Fatal("application persisted without audit")
			}
		})
	}
}
