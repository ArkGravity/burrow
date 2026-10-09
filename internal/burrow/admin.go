package burrow

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func page(r *http.Request) (int, int) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	n, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if p < 1 {
		p = 1
	}
	if n < 1 || n > 100 {
		n = 20
	}
	return p, n
}
func (b *Server) list(w http.ResponseWriter, r *http.Request, resource string) {
	_, _, ok := b.require(w, r, resource+":read")
	if !ok {
		return
	}
	p, n := page(r)
	var out any
	switch resource {
	case "users":
		out = &[]User{}
	case "groups":
		out = &[]Group{}
	case "roles":
		out = &[]Role{}
	case "permissions":
		out = &[]Permission{}
	case "applications":
		out = &[]Application{}
	}
	q := b.DB.Model(out)
	if search := r.URL.Query().Get("search"); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		if resource == "permissions" {
			q = q.Where("LOWER(name) LIKE ? OR application_id IN (SELECT id FROM applications WHERE LOWER(name) LIKE ? OR LOWER(client_id) LIKE ?)", pattern, pattern, pattern)
		} else {
			q = q.Where("LOWER(name) LIKE ?", pattern)
		}
	}
	if status := r.URL.Query().Get("enabled"); status != "" && (resource == "users" || resource == "applications") {
		q = q.Where("enabled = ?", status == "true")
	}
	var total int64
	if q.Count(&total).Error != nil || q.Order("id").Limit(n).Offset((p-1)*n).Find(out).Error != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	switch values := out.(type) {
	case *[]Permission:
		ids := []string{}
		for _, v := range *values {
			if v.ApplicationID != "" {
				ids = append(ids, v.ApplicationID)
			}
		}
		if len(ids) > 0 {
			var apps []ApplicationRef
			if err := b.DB.Model(&Application{}).Select("id", "name", "client_id").Where("id IN ?", ids).Scan(&apps).Error; err != nil {
				fail(w, r, 503, "unavailable")
				return
			}
			refs := map[string]ApplicationRef{}
			for _, app := range apps {
				refs[app.ID] = app
			}
			for i := range *values {
				if app, ok := refs[(*values)[i].ApplicationID]; ok {
					(*values)[i].Application = &app
				}
			}
		}
	case *[]User:
		if err := b.hydrateUsers(b.DB, *values); err != nil {
			fail(w, r, 503, "unavailable")
			return
		}
	case *[]Group:
		for i := range *values {
			v := &(*values)[i]
			v.UserIDs = []string{}
			v.RoleIDs = []string{}
			b.DB.Model(&GroupMember{}).Where("group_id = ?", v.ID).Pluck("user_id", &v.UserIDs)
			b.DB.Model(&GroupRole{}).Where("group_id = ?", v.ID).Pluck("role_id", &v.RoleIDs)
		}
	case *[]Role:
		for i := range *values {
			v := &(*values)[i]
			v.PermissionIDs = []string{}
			b.DB.Model(&RolePermission{}).Where("role_id = ?", v.ID).Pluck("permission_id", &v.PermissionIDs)
		}
	case *[]Application:
		for i := range *values {
			b.hydrateApp(b.DB, &(*values)[i])
		}
	}
	write(w, 200, map[string]any{"items": out, "total": total, "page": p, "pageSize": n})
}
func (b *Server) hydrateUser(tx *gorm.DB, u *User) error {
	users := []User{*u}
	if err := b.hydrateUsers(tx, users); err != nil {
		return err
	}
	*u = users[0]
	return nil
}

// Resolve memberships for the whole page, without requiring groups:read or
// exposing group membership lists and role assignments in the user response.
func (b *Server) hydrateUsers(tx *gorm.DB, users []User) error {
	if len(users) == 0 {
		return nil
	}
	ids := make([]string, len(users))
	index := map[string]int{}
	for i := range users {
		ids[i] = users[i].ID
		index[users[i].ID] = i
		users[i].RoleIDs = []string{}
		users[i].GroupIDs = []string{}
		users[i].Groups = []GroupRef{}
	}
	var roles []UserRole
	if err := tx.Where("user_id IN ?", ids).Order("role_id").Find(&roles).Error; err != nil {
		return err
	}
	for _, role := range roles {
		i := index[role.UserID]
		users[i].RoleIDs = append(users[i].RoleIDs, role.RoleID)
	}
	var groups []struct{ UserID, ID, Name string }
	if err := tx.Table("group_members").Select("group_members.user_id, groups.id, groups.name").Joins("JOIN groups ON groups.id = group_members.group_id").Where("group_members.user_id IN ?", ids).Order("groups.name, groups.id").Scan(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		i := index[group.UserID]
		users[i].GroupIDs = append(users[i].GroupIDs, group.ID)
		users[i].Groups = append(users[i].Groups, GroupRef{ID: group.ID, Name: group.Name})
	}
	return nil
}
func (b *Server) hydrateApp(tx *gorm.DB, a *Application) {
	a.RoleIDs = []string{}
	tx.Model(&RolePermission{}).Where("permission_id = ?", "app:"+a.ID+":login").Pluck("role_id", &a.RoleIDs)
}
func rawString(in map[string]json.RawMessage, k string) string {
	var s string
	json.Unmarshal(in[k], &s)
	return s
}
func rawIDs(in map[string]json.RawMessage, k string) ([]string, error) {
	ids := []string{}
	if e := json.Unmarshal(in[k], &ids); e != nil {
		return nil, invalid(k)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return nil, invalid(k)
		}
		seen[id] = true
	}
	return ids, nil
}
func exists(tx *gorm.DB, model any, id string) error {
	var n int64
	if e := tx.Model(model).Where("id = ?", id).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return invalid("reference")
	}
	return nil
}
func (b *Server) mutate(w http.ResponseWriter, r *http.Request, resource string) {
	permission := resource + ":write"
	if resource == "roles" {
		permission = "authorization:write"
	}
	actor, _, ok := b.require(w, r, permission)
	if !ok {
		return
	}
	if resource == "permissions" {
		fail(w, r, 405, "method_not_allowed")
		return
	}
	in := map[string]json.RawMessage{}
	if r.Method != "DELETE" && !decode(w, r, &in) {
		return
	}
	id := chi.URLParam(r, "id")
	creating := r.Method == "POST"
	if creating {
		id = random(18)
	}
	delete(in, "id")
	delete(in, "builtin")
	allowed := map[string][]string{
		"users":        {"username", "name", "email", "enabled", "mustChangePassword", "language", "theme", "password", "roleIds", "groupIds"},
		"groups":       {"name", "description", "userIds", "roleIds"},
		"roles":        {"name", "description", "permissionIds"},
		"applications": {"name", "clientId", "clientSecret", "clientType", "enabled", "icon", "loginUrl", "redirectUris", "postLogoutRedirectUris", "origins", "allowWithoutPkce", "roleIds"},
	}
	for k := range in {
		if !contains(allowed[resource], k) {
			fail(w, r, 400, "invalid_request")
			return
		}
	}
	p, admin, e := permissions(b.DB, actor)
	if e != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	for _, field := range []string{"roleIds", "groupIds", "userIds", "permissionIds"} {
		if _, found := in[field]; found && !contains(p, "authorization:write") {
			fail(w, r, 403, "forbidden")
			return
		}
	}
	var result any
	auditDetails := ""
	secret := ""
	b.mu.Lock()
	e = b.DB.Transaction(func(tx *gorm.DB) error {
		if b.Config.DBDriver == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(734285622)").Error; err != nil {
				return err
			}
		}
		actor, _, e = b.sessionDB(tx, r)
		if e != nil {
			return errors.New("forbidden")
		}
		p, admin, e = permissions(tx, actor)
		if e != nil {
			return e
		}
		if actor.MustChangePassword || !contains(p, permission) {
			return errors.New("forbidden")
		}
		for _, field := range []string{"roleIds", "groupIds", "userIds", "permissionIds"} {
			if _, found := in[field]; found && !contains(p, "authorization:write") {
				return errors.New("forbidden")
			}
		}
		if r.Method == "DELETE" {
			if err := b.deleteResource(tx, resource, id, admin, contains(p, "authorization:write")); err != nil {
				return err
			}
			if e := enabledAdminExists(tx); e != nil {
				return e
			}
			return tx.Create(&Event{ID: random(18), ActorID: actor.ID, ObjectID: id, Kind: resource + ":" + strings.ToLower(r.Method), RequestID: requestID(r), Success: true, CreatedAt: time.Now()}).Error
		}
		data, _ := json.Marshal(in)
		switch resource {
		case "users":
			u := User{ID: id, Enabled: true, MustChangePassword: true, Language: "en", Theme: "system"}
			if !creating {
				if err := guardAdminTarget(tx, actor, id); err != nil {
					return err
				}
				if err := tx.First(&u, "id = ?", id).Error; err != nil {
					return err
				}
			}
			oldHash := u.PasswordHash
			oldMust := u.MustChangePassword
			if err := json.Unmarshal(data, &u); err != nil {
				return invalid("request")
			}
			u.PasswordHash = oldHash
			if !creating {
				u.MustChangePassword = oldMust
			}
			if strings.TrimSpace(u.Username) == "" || len(u.Username) > 128 {
				return invalid("username")
			}
			if !contains([]string{"en", "zh-CN"}, u.Language) || !contains([]string{"system", "dark", "light"}, u.Theme) {
				return invalid("profile")
			}
			_, passwordProvided := in["password"]
			if creating || passwordProvided {
				h, err := passwordHash(rawString(in, "password"))
				if err != nil {
					return err
				}
				u.PasswordHash = h
				u.MustChangePassword = true
			}
			if u.Enabled && u.PasswordHash == "" {
				return invalid("password")
			}
			if err := tx.Save(&u).Error; err != nil {
				return err
			}
			if err := b.assign(tx, "users", id, in, admin); err != nil {
				return err
			}
			if !u.Enabled || passwordProvided {
				if err := tx.Model(&User{}).Where("id = ?", id).Update("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
					return err
				}
				if err := invalidateAuthentication(tx, id, "", ""); err != nil {
					return err
				}
			}
			if err := b.hydrateUser(tx, &u); err != nil {
				return err
			}
			result = u
		case "groups":
			v := Group{ID: id}
			if !creating {
				if err := tx.First(&v, "id = ?", id).Error; err != nil {
					return err
				}
			}
			if err := json.Unmarshal(data, &v); err != nil {
				return invalid("request")
			}
			if strings.TrimSpace(v.Name) == "" {
				return invalid("name")
			}
			if err := tx.Save(&v).Error; err != nil {
				return err
			}
			if err := b.assign(tx, "groups", id, in, admin); err != nil {
				return err
			}
			result = v
		case "roles":
			v := Role{ID: id}
			if !creating {
				if err := tx.First(&v, "id = ?", id).Error; err != nil {
					return err
				}
				if v.Builtin {
					return errors.New("builtin_role")
				}
			}
			if err := json.Unmarshal(data, &v); err != nil {
				return invalid("request")
			}
			if strings.TrimSpace(v.Name) == "" {
				return invalid("name")
			}
			if err := tx.Save(&v).Error; err != nil {
				return err
			}
			if err := b.assign(tx, "roles", id, in, admin); err != nil {
				return err
			}
			result = v
		case "applications":
			v := Application{ID: id, ClientID: random(18), ClientType: "web", Enabled: true, RedirectURLs: []string{}, LogoutURLs: []string{}, Origins: []string{}}
			if !creating {
				if err := tx.First(&v, "id = ?", id).Error; err != nil {
					return err
				}
			}
			oldClient := v.ClientID
			oldType := v.ClientType
			oldAllowWithoutPKCE := v.AllowWithoutPKCE
			if raw, present := in["clientId"]; present && creating {
				var value string
				if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
					return invalid("client_id")
				}
				if value == "" {
					delete(in, "clientId")
					data, _ = json.Marshal(in)
				} else if len(value) > 128 || strings.IndexFunc(value, func(r rune) bool {
					return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-._~", r))
				}) >= 0 {
					return invalid("client_id")
				}
			}
			if raw, present := in["clientSecret"]; present {
				if !creating {
					return invalid("immutable_client")
				}
				if string(raw) == "null" || json.Unmarshal(raw, &secret) != nil {
					return invalid("client_secret")
				}
				if secret != "" && (len(secret) < 16 || len(secret) > 256 || strings.IndexFunc(secret, func(r rune) bool { return r < 33 || r > 126 }) >= 0) {
					return invalid("client_secret")
				}
			}
			if raw, present := in["allowWithoutPkce"]; present {
				var value bool
				if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
					return invalid("allow_without_pkce")
				}
			}
			if err := json.Unmarshal(data, &v); err != nil {
				return invalid("request")
			}
			if !creating && (v.ClientID != oldClient || v.ClientType != oldType) {
				return invalid("immutable_client")
			}
			if v.AllowWithoutPKCE != oldAllowWithoutPKCE && !admin {
				return errors.New("forbidden")
			}
			if v.AllowWithoutPKCE && v.ClientType != "web" {
				return invalid("allow_without_pkce")
			}
			if v.AllowWithoutPKCE != oldAllowWithoutPKCE {
				details, err := json.Marshal(map[string]any{"allowWithoutPkce": map[string]bool{"before": oldAllowWithoutPKCE, "after": v.AllowWithoutPKCE}})
				if err != nil {
					return err
				}
				auditDetails = string(details)
			}
			if v.Name == "" || v.ClientID == "" || !contains([]string{"web", "spa"}, v.ClientType) || len(v.RedirectURLs) == 0 {
				return invalid("application")
			}
			for _, u := range append(append(append([]string{}, v.RedirectURLs...), v.LogoutURLs...), v.LoginURL) {
				if err := b.validateURL(u, false); err != nil {
					return err
				}
			}
			for _, u := range v.Origins {
				if err := b.validateURL(u, true); err != nil {
					return err
				}
			}
			if v.ClientType == "spa" && secret != "" {
				return invalid("client_secret")
			}
			if creating && v.ClientType == "web" {
				if secret == "" {
					secret = random(32)
				}
				v.SecretHash = hash(secret)
			}
			if err := tx.Save(&v).Error; err != nil {
				return err
			}
			if creating {
				pid := "app:" + id + ":login"
				if err := tx.Create(&Permission{ID: pid, Name: pid, ApplicationID: id, Description: "Login to " + v.Name}).Error; err != nil {
					return err
				}
			}
			if err := b.assign(tx, "applications", id, in, admin); err != nil {
				return err
			}
			b.hydrateApp(tx, &v)
			result = v
		}
		if e := enabledAdminExists(tx); e != nil {
			return e
		}
		return tx.Create(&Event{ID: random(18), ActorID: actor.ID, ObjectID: id, Kind: resource + ":" + strings.ToLower(r.Method), RequestID: requestID(r), Details: auditDetails, Success: true, CreatedAt: time.Now()}).Error
	})
	b.mu.Unlock()
	if e != nil {
		status := 400
		code := e.Error()
		if errors.Is(e, gorm.ErrRecordNotFound) {
			status = 404
			code = "not_found"
		} else if errors.Is(e, gorm.ErrDuplicatedKey) {
			status = 409
			code = "conflict"
		} else if code == "forbidden" {
			status = 403
		} else if contains([]string{"in_use", "last_admin", "builtin_role"}, code) {
			status = 409
		} else if !strings.HasPrefix(code, "invalid_") && !contains([]string{"last_admin", "builtin_role", "forbidden", "in_use", "password_policy"}, code) {
			status = 503
			code = "unavailable"
		}
		fail(w, r, status, code)
		return
	}
	if r.Method == "DELETE" {
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	if secret != "" {
		data, _ := json.Marshal(result)
		var out map[string]any
		json.Unmarshal(data, &out)
		out["clientSecret"] = secret
		result = out
	}
	status := 200
	if creating {
		status = 201
	}
	write(w, status, result)
}
func (b *Server) validateURL(raw string, origin bool) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || strings.Contains(raw, "*") || (u.Scheme != "https" && !(b.Config.Env == "dev" && u.Scheme == "http")) {
		return invalid("url")
	}
	if origin && (u.Path != "" || u.RawQuery != "") {
		return invalid("origin")
	}
	return nil
}
func (b *Server) assign(tx *gorm.DB, resource, id string, in map[string]json.RawMessage, admin bool) error {
	if raw, ok := in["roleIds"]; ok {
		ids, e := rawIDs(in, "roleIds")
		if e != nil {
			return e
		}
		_ = raw
		old := []string{}
		switch resource {
		case "users":
			tx.Model(&UserRole{}).Where("user_id = ?", id).Pluck("role_id", &old)
		case "groups":
			tx.Model(&GroupRole{}).Where("group_id = ?", id).Pluck("role_id", &old)
		case "applications":
			tx.Model(&RolePermission{}).Where("permission_id = ?", "app:"+id+":login").Pluck("role_id", &old)
		default:
			return invalid("roleIds")
		}
		if !admin && contains(old, "admin") != contains(ids, "admin") {
			return errors.New("forbidden")
		}
		for _, role := range ids {
			if err := exists(tx, &Role{}, role); err != nil {
				return err
			}
		}
		switch resource {
		case "users":
			if e := tx.Where("user_id = ?", id).Delete(&UserRole{}).Error; e != nil {
				return e
			}
			for _, role := range ids {
				if e := tx.Create(&UserRole{UserID: id, RoleID: role}).Error; e != nil {
					return e
				}
			}
		case "groups":
			if e := tx.Where("group_id = ?", id).Delete(&GroupRole{}).Error; e != nil {
				return e
			}
			for _, role := range ids {
				if e := tx.Create(&GroupRole{GroupID: id, RoleID: role}).Error; e != nil {
					return e
				}
			}
		case "applications":
			for _, roleID := range ids {
				var role Role
				if err := tx.First(&role, "id = ?", roleID).Error; err != nil {
					return err
				}
				if role.Builtin {
					return errors.New("builtin_role")
				}
			}
			if e := tx.Where("permission_id = ?", "app:"+id+":login").Delete(&RolePermission{}).Error; e != nil {
				return e
			}
			for _, role := range ids {
				if e := tx.Create(&RolePermission{RoleID: role, PermissionID: "app:" + id + ":login"}).Error; e != nil {
					return e
				}
			}
		}
	}
	if _, ok := in["groupIds"]; ok {
		if resource != "users" {
			return invalid("groupIds")
		}
		ids, e := rawIDs(in, "groupIds")
		if e != nil {
			return e
		}
		old := []string{}
		tx.Model(&GroupMember{}).Where("user_id = ?", id).Pluck("group_id", &old)
		for _, gid := range append(append([]string{}, old...), ids...) {
			if err := exists(tx, &Group{}, gid); err != nil {
				return err
			}
			if !admin {
				var count int64
				tx.Model(&GroupRole{}).Where("group_id = ? AND role_id = ?", gid, "admin").Count(&count)
				if count > 0 && contains(old, gid) != contains(ids, gid) {
					return errors.New("forbidden")
				}
			}
		}
		if e := tx.Where("user_id = ?", id).Delete(&GroupMember{}).Error; e != nil {
			return e
		}
		for _, gid := range ids {
			if e := tx.Create(&GroupMember{GroupID: gid, UserID: id}).Error; e != nil {
				return e
			}
		}
	}
	if _, ok := in["userIds"]; ok {
		if resource != "groups" {
			return invalid("userIds")
		}
		ids, e := rawIDs(in, "userIds")
		if e != nil {
			return e
		}
		if !admin {
			var count int64
			tx.Model(&GroupRole{}).Where("group_id = ? AND role_id = ?", id, "admin").Count(&count)
			if count > 0 {
				return errors.New("forbidden")
			}
		}
		for _, uid := range ids {
			if e := exists(tx, &User{}, uid); e != nil {
				return e
			}
		}
		if e := tx.Where("group_id = ?", id).Delete(&GroupMember{}).Error; e != nil {
			return e
		}
		for _, uid := range ids {
			if e := tx.Create(&GroupMember{GroupID: id, UserID: uid}).Error; e != nil {
				return e
			}
		}
	}
	if _, ok := in["permissionIds"]; ok {
		if resource != "roles" {
			return invalid("permissionIds")
		}
		ids, e := rawIDs(in, "permissionIds")
		if e != nil {
			return e
		}
		for _, pid := range ids {
			if e := exists(tx, &Permission{}, pid); e != nil {
				return e
			}
		}
		if e := tx.Where("role_id = ?", id).Delete(&RolePermission{}).Error; e != nil {
			return e
		}
		for _, pid := range ids {
			if e := tx.Create(&RolePermission{RoleID: id, PermissionID: pid}).Error; e != nil {
				return e
			}
		}
	}
	return nil
}
func (b *Server) deleteResource(tx *gorm.DB, resource, id string, admin, canAuthorize bool) error {
	count := func(model any, where string) bool {
		var n int64
		err := tx.Model(model).Where(where, id).Count(&n).Error
		return err != nil || n > 0
	}
	switch resource {
	case "groups":
		if count(&GroupMember{}, "group_id = ?") || count(&GroupRole{}, "group_id = ?") {
			return errors.New("in_use")
		}
		return tx.Delete(&Group{}, "id = ?", id).Error
	case "roles":
		if id == "admin" {
			return errors.New("builtin_role")
		}
		if count(&UserRole{}, "role_id = ?") || count(&GroupRole{}, "role_id = ?") || count(&RolePermission{}, "role_id = ?") {
			return errors.New("in_use")
		}
		return tx.Delete(&Role{}, "id = ?", id).Error
	case "users":
		ids, e := roleIDs(tx, id)
		if e != nil {
			return e
		}
		if !canAuthorize && len(ids) > 0 {
			return errors.New("forbidden")
		}
		if contains(ids, "admin") && !admin {
			return errors.New("forbidden")
		}
		for _, m := range []any{&UserRole{}, &GroupMember{}, &LoginTransaction{}} {
			if e := tx.Where("user_id = ?", id).Delete(m).Error; e != nil {
				return e
			}
		}
		if e := tx.Model(&Session{}).Where("user_id = ?", id).Update("revoked", true).Error; e != nil {
			return e
		}
		if e := tx.Model(&TokenRecord{}).Where("user_id = ?", id).Update("revoked", true).Error; e != nil {
			return e
		}
		if e := tx.Where("user_id = ?", id).Delete(&AuthTransaction{}).Error; e != nil {
			return e
		}
		return tx.Delete(&User{}, "id = ?", id).Error
	case "applications":
		var grants int64
		if e := tx.Model(&RolePermission{}).Where("permission_id = ?", "app:"+id+":login").Count(&grants).Error; e != nil {
			return e
		}
		if !canAuthorize && grants > 0 {
			return errors.New("forbidden")
		}
		if e := tx.Where("permission_id = ?", "app:"+id+":login").Delete(&RolePermission{}).Error; e != nil {
			return e
		}
		if e := tx.Where("application_id = ?", id).Delete(&Permission{}).Error; e != nil {
			return e
		}
		if e := tx.Where("client_id = ?", id).Delete(&AuthTransaction{}).Error; e != nil {
			return e
		}
		if e := tx.Model(&TokenRecord{}).Where("client_id = ?", id).Update("revoked", true).Error; e != nil {
			return e
		}
		return tx.Delete(&Application{}, "id = ?", id).Error
	}
	return invalid("resource")
}
func (b *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	_, _, ok := b.require(w, r, "users:write")
	if !ok {
		return
	}
	var in struct{ Password string }
	if !decode(w, r, &in) {
		return
	}
	h, e := passwordHash(in.Password)
	if e != nil {
		fail(w, r, 400, "password_policy")
		return
	}
	id := chi.URLParam(r, "id")
	e = b.authorizationTx(r, "users:write", func(tx *gorm.DB, current User, _ Session) error {
		if e := guardAdminTarget(tx, current, id); e != nil {
			return e
		}
		if e := exists(tx, &User{}, id); e != nil {
			return e
		}
		if e := tx.Model(&User{}).Where("id = ?", id).Updates(map[string]any{"password_hash": h, "must_change_password": true, "auth_version": gorm.Expr("auth_version + 1")}).Error; e != nil {
			return e
		}
		return invalidateAuthentication(tx, id, "", "")
	})
	if e != nil {
		mutationError(w, r, e)
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}
func (b *Server) resetSecret(w http.ResponseWriter, r *http.Request) {
	_, _, ok := b.require(w, r, "applications:write")
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	secret := random(32)
	err := b.authorizationTx(r, "applications:write", func(tx *gorm.DB, _ User, _ Session) error {
		var app Application
		if e := tx.First(&app, "id = ?", id).Error; e != nil {
			return e
		}
		if app.ClientType != "web" {
			return invalid("application")
		}
		return tx.Model(&app).Update("secret_hash", hash(secret)).Error
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	write(w, 200, map[string]string{"clientSecret": secret})
}

func guardAdminTarget(tx *gorm.DB, actor User, target string) error {
	_, admin, e := permissions(tx, actor)
	if e != nil {
		return e
	}
	ids, e := roleIDs(tx, target)
	if e != nil {
		return e
	}
	if !admin && contains(ids, "admin") {
		return errors.New("forbidden")
	}
	return nil
}
