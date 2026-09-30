package burrow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSeedIsIdempotentAndPreservesAdministrator(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := testStore(t, driver)
			options := BootstrapConfig{Username: "admin", Password: testPassword, Name: "Administrator"}
			if err := s.Seed(options); err != nil {
				t.Fatal(err)
			}
			var user User
			s.DB.First(&user, "username = ?", "admin")
			if !user.MustChangePassword || !passwordOK(user.PasswordHash, testPassword) {
				t.Fatal("seeded credentials or forced password change missing")
			}
			s.DB.Model(&user).Updates(map[string]any{"name": "Renamed", "enabled": false, "must_change_password": false})
			options.Password = "different-initial-password"
			if err := s.Seed(options); err != nil {
				t.Fatal(err)
			}
			var after User
			s.DB.First(&after, "id = ?", user.ID)
			if after.PasswordHash != user.PasswordHash || after.Name != "Renamed" || after.Enabled || after.MustChangePassword {
				t.Fatal("seed overwrote administrator")
			}
			var count int64
			s.DB.Model(&User{}).Count(&count)
			if count != 1 {
				t.Fatal("seed duplicated users")
			}
			s.DB.Model(&Role{}).Where("builtin = ?", true).Count(&count)
			if count != 3 {
				t.Fatal("default roles missing")
			}
			s.DB.Model(&RolePermission{}).Where("role_id = ?", "viewer").Count(&count)
			if count != 0 {
				t.Fatal("Viewer has management or application permissions")
			}
		})
	}
}

func TestSeedRejectsUsernameCollisionAndProductionExamplePassword(t *testing.T) {
	s := testStore(t, "sqlite")
	u := User{ID: random(18), Username: "admin", Enabled: true}
	if err := s.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(BootstrapConfig{Username: "admin", Password: testPassword}); err == nil {
		t.Fatal("seed promoted an existing ordinary account")
	}
	var count int64
	s.DB.Model(&UserRole{}).Where("user_id = ?", u.ID).Count(&count)
	if count != 0 {
		t.Fatal("collision changed account roles")
	}
	s = testStore(t, "sqlite")
	s.Config.Env = "prod"
	for _, password := range []string{"", developmentAdminPassword} {
		if err := s.Seed(BootstrapConfig{Username: "admin", Password: password}); err == nil {
			t.Fatal("production accepted missing/example password")
		}
	}
	if err := s.Seed(BootstrapConfig{Username: "admin", Password: testPassword}); err != nil {
		t.Fatal(err)
	}
}

func TestNewUsersDefaultToViewerAndCanExplicitlyChooseRoles(t *testing.T) {
	b, c, _ := testServer(t, "sqlite")
	for _, example := range []struct {
		username  string
		roles     any
		specified bool
		expected  []string
	}{
		{"default-viewer", nil, false, []string{"viewer"}},
		{"explicit-editor", []string{"editor"}, true, []string{"editor"}},
		{"no-roles", []string{}, true, []string{}},
	} {
		body := map[string]any{"username": example.username, "name": example.username, "localEnabled": false}
		if example.specified {
			body["roleIds"] = example.roles
		}
		w := c.request("POST", "/api/v1/users", body, true)
		if w.Code != 201 {
			t.Fatalf("create: %d %s", w.Code, w.Body.String())
		}
		var user User
		if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
			t.Fatal(err)
		}
		if strings.Join(user.RoleIDs, ",") != strings.Join(example.expected, ",") {
			t.Fatal("wrong default role")
		}
		w = c.request("PUT", "/api/v1/users/"+user.ID, map[string]any{"name": "updated"}, true)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var updated User
		json.Unmarshal(w.Body.Bytes(), &updated)
		if strings.Join(updated.RoleIDs, ",") != strings.Join(example.expected, ",") {
			t.Fatal("edit changed roles")
		}
	}
	// Only users:write is required for the implicit Viewer default; explicit
	// authorization changes still require authorization:write.
	h, _ := passwordHash(testPassword)
	operator := User{ID: random(18), Username: "provisioner", Enabled: true, LocalEnabled: true, PasswordHash: h}
	b.DB.Create(&operator)
	role := Role{ID: random(18), Name: "provisioner"}
	b.DB.Create(&role)
	b.DB.Create(&RolePermission{RoleID: role.ID, PermissionID: "users:write"})
	b.DB.Create(&UserRole{UserID: operator.ID, RoleID: role.ID})
	limited := newBrowser(b)
	limited.login(t, operator.Username, testPassword)
	if w := limited.request("POST", "/api/v1/users", map[string]any{"username": "implicit-viewer", "localEnabled": false}, true); w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	if w := limited.request("POST", "/api/v1/users", map[string]any{"username": "escalate", "localEnabled": false, "roleIds": []string{"editor"}}, true); w.Code != 403 {
		t.Fatal("provisioner assigned explicit roles")
	}
}

func TestDefaultRoleBoundariesAndUserGroupSummaries(t *testing.T) {
	b, admin, _ := testServer(t, "sqlite")
	h, _ := passwordHash(testPassword)
	for _, role := range []string{"viewer", "editor"} {
		u := User{ID: random(18), Username: role, Enabled: true, LocalEnabled: true, PasswordHash: h}
		b.DB.Create(&u)
		b.DB.Create(&UserRole{UserID: u.ID, RoleID: role})
		c := newBrowser(b)
		c.login(t, u.Username, testPassword)
		if w := c.request("GET", "/api/v1/me", nil, false); w.Code != 200 {
			t.Fatal("personal profile denied")
		}
		if w := c.request("GET", "/api/v1/me/apps", nil, false); w.Code != 200 {
			t.Fatal("personal portal denied")
		}
		if w := c.request("POST", "/api/v1/users", map[string]any{"username": "denied"}, true); w.Code != 403 {
			t.Fatal("default role manages users")
		}
		if w := c.request("POST", "/api/v1/roles", map[string]any{"name": "denied"}, true); w.Code != 403 {
			t.Fatal("default role manages authorization")
		}
		if role == "viewer" {
			for _, resource := range []string{"users", "groups", "roles", "permissions", "applications", "providers"} {
				if w := c.request("GET", "/api/v1/"+resource, nil, false); w.Code != 403 {
					t.Fatal("Viewer reads management resources")
				}
			}
		} else {
			if w := c.request("POST", "/api/v1/providers", map[string]any{"name": "upstream", "issuer": "https://issuer.example", "clientId": "rp", "clientSecret": "test-only"}, true); w.Code != 201 {
				t.Fatal(w.Body.String())
			}
		}
	}
	var viewer User
	b.DB.First(&viewer, "username = ?", "viewer")
	group := Group{ID: random(18), Name: "Engineering"}
	b.DB.Create(&group)
	b.DB.Create(&GroupMember{GroupID: group.ID, UserID: viewer.ID})
	w := admin.request("GET", "/api/v1/users", nil, false)
	var list struct {
		Items []User `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, u := range list.Items {
		if u.ID == viewer.ID && (len(u.Groups) != 1 || u.Groups[0].Name != "Engineering") {
			t.Fatal("group names absent from user list")
		}
	}
	// Groups displayed on a user are part of users:read, not an implicit grant
	// to inspect every group's members and authorization settings.
	reader := User{ID: random(18), Username: "user-reader", Enabled: true, LocalEnabled: true, PasswordHash: h}
	b.DB.Create(&reader)
	readRole := Role{ID: random(18), Name: "User reader"}
	b.DB.Create(&readRole)
	b.DB.Create(&RolePermission{RoleID: readRole.ID, PermissionID: "users:read"})
	b.DB.Create(&UserRole{UserID: reader.ID, RoleID: readRole.ID})
	readBrowser := newBrowser(b)
	readBrowser.login(t, reader.Username, testPassword)
	if w := readBrowser.request("GET", "/api/v1/groups", nil, false); w.Code != 403 {
		t.Fatal("reader accessed all groups")
	}
	w = readBrowser.request("GET", "/api/v1/users", nil, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Engineering") {
		t.Fatal("reader cannot see user group names")
	}
	for _, role := range []string{"viewer", "editor"} {
		app := testApp(t, b, "spa")
		if w := admin.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"roleIds": []string{role}}, true); w.Code != 409 {
			t.Fatal("built-in role received blanket application access")
		}
	}
}
