package burrow

import (
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The first body read occurs after the initial session/permission check. This
// models a client that pauses sending JSON while another request changes access.
type interleavedBody struct {
	io.Reader
	once   sync.Once
	change func()
}

func (r *interleavedBody) Read(p []byte) (int, error) { r.once.Do(r.change); return r.Reader.Read(p) }
func interleave(c *browser, method, path, body string, change func()) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, &interleavedBody{Reader: strings.NewReader(body), change: change})
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", c.csrf)
	for _, cookie := range c.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	c.b.ServeHTTP(w, r)
	return w
}
func TestProfileCannotRestoreDisabledAccount(t *testing.T) {
	b, c, u := testServer(t, "sqlite")
	w := interleave(c, "PUT", "/api/v1/me", `{"name":"updated"}`, func() {
		if err := b.DB.Model(&User{}).Where("id = ?", u.ID).Update("enabled", false).Error; err != nil {
			t.Fatal(err)
		}
	})
	var current User
	b.DB.First(&current, "id = ?", u.ID)
	if current.Enabled {
		t.Fatal("profile update restored disabled user")
	}
	if w.Code == 200 {
		t.Fatal("revoked actor may not update profile")
	}
}
func TestPasswordCannotOverwriteConcurrentReset(t *testing.T) {
	b, c, u := testServer(t, "sqlite")
	replacement, err := passwordHash("replacement-password-2026")
	if err != nil {
		t.Fatal(err)
	}
	w := interleave(c, "POST", "/api/v1/me/password", `{"currentPassword":"`+testPassword+`-changed","password":"attacker-new-password"}`, func() { b.DB.Model(&User{}).Where("id = ?", u.ID).Update("password_hash", replacement) })
	var current User
	b.DB.First(&current, "id = ?", u.ID)
	if current.PasswordHash != replacement {
		t.Fatal("stale current password overwrote admin reset")
	}
	if w.Code == 200 {
		t.Fatal("concurrent reset accepted stale password")
	}
}
func TestPasswordCannotResetPromotedAdmin(t *testing.T) {
	b, _, _ := testServer(t, "sqlite")
	h, _ := passwordHash(testPassword)
	operator := User{ID: random(18), Username: "operator", Enabled: true, PasswordHash: h}
	target := User{ID: random(18), Username: "target", Enabled: true, PasswordHash: h}
	for _, u := range []User{operator, target} {
		if err := b.DB.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	role := Role{ID: random(18), Name: "user-manager"}
	b.DB.Create(&role)
	b.DB.Create(&RolePermission{RoleID: role.ID, PermissionID: "users:write"})
	b.DB.Create(&UserRole{UserID: operator.ID, RoleID: role.ID})
	c := newBrowser(b)
	c.login(t, operator.Username, testPassword)
	w := interleave(c, "PUT", "/api/v1/users/"+target.ID+"/password", `{"password":"attacker-password-2026"}`, func() {
		if err := b.DB.Create(&UserRole{UserID: target.ID, RoleID: "admin"}).Error; err != nil {
			t.Fatal(err)
		}
	})
	var current User
	if err := b.DB.First(&current, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.PasswordHash != h || w.Code != 403 {
		t.Fatalf("promoted admin password reset: status=%d", w.Code)
	}
}

func TestAuditFailureRollsBackMutation(t *testing.T) {
	b, c, _ := testServer(t, "sqlite")
	if e := b.DB.Migrator().DropTable(&Event{}); e != nil {
		t.Fatal(e)
	}
	w := c.request("POST", "/api/v1/groups", map[string]string{"name": "must-not-commit"}, true)
	var n int64
	b.DB.Model(&Group{}).Where("name = ?", "must-not-commit").Count(&n)
	if w.Code == 201 || n != 0 {
		t.Fatal("management change committed without required audit event")
	}
}

func TestAuditFailureRollsBackSensitiveActions(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, u := testServer(t, driver)
			app := testApp(t, b, "web")
			spa := testApp(t, b, "spa")
			if w := c.request("POST", "/api/v1/applications/"+spa.ID+"/secret", nil, true); w.Code != 400 {
				t.Fatalf("SPA secret rotation must be rejected as invalid input: %d", w.Code)
			}
			if e := b.DB.Migrator().DropTable(&Event{}); e != nil {
				t.Fatal(e)
			}
			w := c.request("POST", "/api/v1/applications/"+app.ID+"/secret", nil, true)
			var current Application
			if e := b.DB.First(&current, "id = ?", app.ID).Error; e != nil {
				t.Fatal(e)
			}
			if w.Code != 503 || current.SecretHash != app.SecretHash {
				t.Fatal("secret rotation committed without audit")
			}
			w = c.request("PUT", "/api/v1/users/"+u.ID+"/password", map[string]string{"password": "reset-password-2026"}, true)
			var currentUser User
			if e := b.DB.First(&currentUser, "id = ?", u.ID).Error; e != nil {
				t.Fatal(e)
			}
			if w.Code != 503 || currentUser.PasswordHash != u.PasswordHash || currentUser.AuthVersion != u.AuthVersion || currentUser.MustChangePassword != u.MustChangePassword || c.request("GET", "/api/v1/me", nil, false).Code != 200 {
				t.Fatal("password reset or session invalidation committed without audit")
			}
		})
	}
}

func TestRemovedUserSessionRevocationEndpoint(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			_, c, u := testServer(t, driver)
			w := c.request("POST", "/api/v1/users/"+u.ID+"/revoke-sessions", nil, true)
			if w.Code != 404 {
				t.Fatalf("removed session revocation endpoint returned %d", w.Code)
			}
			if c.request("GET", "/api/v1/me", nil, false).Code != 200 {
				t.Fatal("removed endpoint changed the user's active session")
			}
		})
	}
}
