package burrow

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestApplicationPermissionLabels(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, admin, _ := testServer(t, driver)
			app := testApp(t, b, "spa")
			other := testApp(t, b, "spa")
			if err := b.DB.Model(&app).Update("name", "Grafana").Error; err != nil {
				t.Fatal(err)
			}
			if err := b.DB.Model(&other).Update("name", "Grafana").Error; err != nil {
				t.Fatal(err)
			}
			role := Role{ID: random(18), Name: "Permission reader"}
			b.DB.Create(&role)
			b.DB.Create(&RolePermission{RoleID: role.ID, PermissionID: "permissions:read"})
			h, _ := passwordHash(testPassword)
			user := User{ID: random(18), Username: "reader", PasswordHash: h, Enabled: true}
			b.DB.Create(&user)
			b.DB.Create(&UserRole{UserID: user.ID, RoleID: role.ID})
			reader := newBrowser(b)
			reader.login(t, user.Username, testPassword)
			if out := reader.request("GET", "/api/v1/applications", nil, false); out.Code != 403 {
				t.Fatal("unexpected application read permission")
			}
			for _, search := range []string{"Grafana", app.ClientID} {
				out := reader.request("GET", "/api/v1/permissions?search="+url.QueryEscape(search), nil, false)
				var list struct {
					Items []Permission `json:"items"`
					Total int64        `json:"total"`
				}
				if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &list) != nil {
					t.Fatal("permission search failed", out.Code, out.Body.String())
				}
				want := int64(2)
				if search == app.ClientID {
					want = 1
				}
				if list.Total != want || int64(len(list.Items)) != want {
					t.Fatal("wrong permission search results")
				}
				for _, permission := range list.Items {
					if permission.Application == nil || permission.Application.Name != "Grafana" || permission.Application.ClientID == "" || permission.ID != "app:"+permission.Application.ID+":login" {
						t.Fatal("compact application reference missing")
					}
				}
				if strings.Contains(out.Body.String(), "secretHash") || strings.Contains(out.Body.String(), "redirectUris") {
					t.Fatal("permission read exposes full app configuration")
				}
			}
			b.DB.Create(&RolePermission{RoleID: role.ID, PermissionID: "app:" + app.ID + ":login"})
			before, _, _ := permissions(b.DB, user)
			if out := admin.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"name": "Renamed Grafana"}, true); out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			out := reader.request("GET", "/api/v1/permissions?search="+url.QueryEscape("Renamed Grafana"), nil, false)
			if out.Code != 200 || !strings.Contains(out.Body.String(), "Renamed Grafana") {
				t.Fatal("renamed display not refreshed")
			}
			after, _, _ := permissions(b.DB, user)
			if strings.Join(before, ",") != strings.Join(after, ",") {
				t.Fatal("rename changed permission identifiers")
			}
			code, _ := authorize(t, reader, app, nil)
			if strings.HasPrefix(code, "error:") || strings.HasPrefix(code, "/") {
				t.Fatal("rename revoked app login")
			}
		})
	}
}
