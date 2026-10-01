package burrow

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zitadel/oidc/v3/pkg/op"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type contextKey string

const requestKey contextKey = "request"

type Server struct {
	*Store
	OP        *op.Provider
	router    http.Handler
	limits    sync.Mutex
	attempts  map[string]attempt
	dummyHash string
	assets    fs.FS
}
type attempt struct {
	Count int
	Until time.Time
}

func NewServer(s *Store, assets ...fs.FS) (*Server, error) {
	if err := s.Health(context.Background()); err != nil {
		return nil, err
	}
	h, _ := passwordHash(random(32))
	b := &Server{Store: s, attempts: map[string]attempt{}, dummyHash: h}
	if _, err := (oidcStore{b}).SigningKey(context.Background()); err != nil {
		return nil, fmt.Errorf("signing key unavailable; run migrate and verify master key: %w", err)
	}
	if len(assets) > 0 {
		b.assets = assets[0]
	}
	if b.assets == nil {
		b.assets = os.DirFS(s.Config.StaticDir)
	}
	if err := b.initOIDC(); err != nil {
		return nil, err
	}
	r := chi.NewRouter()
	r.Use(b.security)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Health(r.Context()); err != nil {
			fail(w, r, 503, "not_ready")
			return
		}
		write(w, 200, map[string]string{"status": "ok"})
	})
	r.Get("/api/v1/auth/csrf", b.csrf)
	r.Post("/api/v1/auth/login", b.login)
	r.Post("/api/v1/auth/logout", b.logout)
	r.Get("/api/v1/auth/providers", b.publicProviders)
	r.Get("/api/v1/auth/context", b.authContext)
	r.Get("/api/v1/auth/providers/{id}/login", b.upstreamLogin)
	r.Get("/api/v1/auth/providers/{id}/callback", b.upstreamCallback)
	r.Get("/api/v1/me", b.me)
	r.Put("/api/v1/me", b.profile)
	r.Post("/api/v1/me/password", b.changePassword)
	r.Get("/api/v1/me/apps", b.myApps)
	r.Get("/api/v1/dashboard", b.dashboard)
	for _, resource := range []string{"users", "groups", "roles", "permissions", "applications", "providers"} {
		resource := resource
		r.Get("/api/v1/"+resource, func(w http.ResponseWriter, r *http.Request) { b.list(w, r, resource) })
		r.Post("/api/v1/"+resource, func(w http.ResponseWriter, r *http.Request) { b.mutate(w, r, resource) })
		r.Put("/api/v1/"+resource+"/{id}", func(w http.ResponseWriter, r *http.Request) { b.mutate(w, r, resource) })
		r.Delete("/api/v1/"+resource+"/{id}", func(w http.ResponseWriter, r *http.Request) { b.mutate(w, r, resource) })
	}
	r.Put("/api/v1/users/{id}/password", b.resetPassword)
	r.Post("/api/v1/users/{id}/revoke-sessions", b.revokeSessions)
	r.Get("/api/v1/users/{id}/identities", b.identities)
	r.Post("/api/v1/users/{id}/identities", b.identities)
	r.Delete("/api/v1/users/{id}/identities/{identityId}", b.identities)
	r.Post("/api/v1/applications/{id}/secret", b.resetSecret)
	r.Get("/.well-known/openid-configuration", b.discovery)
	r.Options("/.well-known/openid-configuration", b.discovery)
	r.Get("/oidc/login", b.oidcLogin)
	r.Get("/oidc/authorize", b.protocol)
	r.Post("/oidc/token", b.protocol)
	r.Options("/oidc/token", b.protocol)
	r.Get("/oidc/userinfo", b.protocol)
	r.Options("/oidc/userinfo", b.protocol)
	r.Post("/oidc/userinfo", b.protocol)
	r.Get("/oidc/jwks", b.protocol)
	r.Options("/oidc/jwks", b.protocol)
	r.Get("/oidc/authorize/callback", b.protocol)
	r.Get("/oidc/logout", b.oidcLogout)
	r.Post("/oidc/logout", b.oidcLogout)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/oidc/") || strings.HasPrefix(r.URL.Path, "/.well-known/") || (r.Method != "GET" && r.Method != "HEAD") {
			fail(w, r, 404, "not_found")
			return
		}
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if st, e := fs.Stat(b.assets, p); e != nil || st.IsDir() {
			p = "index.html"
		}
		if _, e := fs.Stat(b.assets, p); e != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFileFS(w, r, b.assets, p)
	})
	b.router = r
	return b, nil
}
func (b *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { b.router.ServeHTTP(w, r) }
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, r *http.Request, status int, code string) {
	write(w, status, map[string]any{"error": map[string]any{"code": code, "requestId": r.Context().Value(requestKey)}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(v); err != nil {
		fail(w, r, 400, "invalid_request")
		return false
	}
	return true
}
func (b *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := random(12)
		r = r.WithContext(context.WithValue(r.Context(), requestKey, id))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		if b.Config.Env != "dev" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" && r.Method != "HEAD" {
			if !b.validCSRF(r) {
				fail(w, r, 403, "csrf_invalid")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (b *Server) cookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: b.Config.Env != "dev", SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func (b *Server) browser(w http.ResponseWriter, r *http.Request) string {
	if c, e := r.Cookie("burrow_browser"); e == nil && len(c.Value) == 43 {
		return c.Value
	}
	v := random(32)
	b.cookie(w, "burrow_browser", v, 36000)
	return v
}
func (b *Server) csrfToken(browser string) string {
	m := hmac.New(sha256.New, b.Config.MasterKey[:])
	m.Write([]byte("csrf:" + browser))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (b *Server) csrf(w http.ResponseWriter, r *http.Request) {
	v := b.browser(w, r)
	write(w, 200, map[string]string{"token": b.csrfToken(v)})
}
func (b *Server) validCSRF(r *http.Request) bool {
	c, e := r.Cookie("burrow_browser")
	if e != nil {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != b.Config.Issuer {
		return false
	}
	return hmac.Equal([]byte(r.Header.Get("X-CSRF-Token")), []byte(b.csrfToken(c.Value)))
}
func (b *Server) session(r *http.Request) (User, Session, error) { return b.sessionDB(b.DB, r) }
func (b *Server) require(w http.ResponseWriter, r *http.Request, permission string) (User, Session, bool) {
	u, s, e := b.session(r)
	if e != nil {
		fail(w, r, 401, "unauthenticated")
		return u, s, false
	}
	if permission != "" {
		if u.MustChangePassword {
			fail(w, r, 403, "password_change_required")
			return u, s, false
		}
		p, _, e := permissions(b.DB, u)
		if e != nil {
			fail(w, r, 503, "unavailable")
			return u, s, false
		}
		if !contains(p, permission) {
			fail(w, r, 403, "forbidden")
			return u, s, false
		}
	}
	return u, s, true
}
func (b *Server) meResult(u User) (map[string]any, error) {
	p, administrator, err := permissions(b.DB, u)
	if err != nil {
		return nil, err
	}
	if u.MustChangePassword {
		p = []string{}
		administrator = false
	}
	return map[string]any{"user": u, "permissions": p, "administrator": administrator}, nil
}
func (b *Server) me(w http.ResponseWriter, r *http.Request) {
	u, _, ok := b.require(w, r, "")
	if ok {
		out, err := b.meResult(u)
		if err != nil {
			fail(w, r, 503, "unavailable")
			return
		}
		write(w, 200, out)
	}
}
func (b *Server) limited(r *http.Request, bucket string) bool {
	ip := b.clientIP(r)
	b.limits.Lock()
	defer b.limits.Unlock()
	now := time.Now()
	for k, v := range b.attempts {
		if v.Until.Before(now) {
			delete(b.attempts, k)
		}
	}
	key := bucket + ip
	a := b.attempts[key]
	if a.Until.Before(now) {
		a = attempt{Until: now.Add(time.Minute)}
	}
	a.Count++
	b.attempts[key] = a
	return a.Count > 30
}
func (b *Server) newSession(w http.ResponseWriter, r *http.Request, u User, method, provider string, subject ...string) error {
	return b.newSessionAt(w, r, u, method, provider, time.Now(), subject...)
}
func (b *Server) newSessionAt(w http.ResponseWriter, r *http.Request, u User, method, provider string, authTime time.Time, subject ...string) error {
	credential := random(32)
	s := Session{ID: random(18), UserID: u.ID, CredentialHash: hash(credential), Method: method, ProviderID: provider, AuthTime: authTime, ExpiresAt: time.Now().Add(b.Config.SessionTTL)}
	err := b.DB.Transaction(func(tx *gorm.DB) error {
		var current User
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND enabled = ?", u.ID, true).First(&current).Error; e != nil {
			return e
		}
		if method == "password" && (!current.LocalEnabled || current.PasswordHash != u.PasswordHash) {
			return fmt.Errorf("authentication changed")
		}
		if method == "oidc" {
			var p Provider
			if e := tx.Where("id = ? AND enabled = ?", provider, true).First(&p).Error; e != nil {
				return e
			}
			if len(subject) != 1 {
				return fmt.Errorf("external subject missing")
			}
			var n int64
			if e := tx.Model(&ExternalIdentity{}).Where("user_id = ? AND provider_id = ? AND issuer = ? AND subject = ?", u.ID, p.ID, p.Issuer, subject[0]).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return fmt.Errorf("external identity changed")
			}
		}
		if c, e := r.Cookie("burrow_session"); e == nil {
			if e := tx.Model(&Session{}).Where("credential_hash = ?", hash(c.Value)).Update("revoked", true).Error; e != nil {
				return e
			}
		}
		return tx.Create(&s).Error
	})
	if err != nil {
		return err
	}
	b.cookie(w, "burrow_session", credential, int(b.Config.SessionTTL.Seconds()))
	return nil
}
func (b *Server) login(w http.ResponseWriter, r *http.Request) {
	if b.limited(r, "login") {
		fail(w, r, 429, "rate_limited")
		return
	}
	var in struct{ Username, Password, RequestID string }
	if !decode(w, r, &in) {
		return
	}
	var u User
	e := b.DB.Where("username = ?", in.Username).First(&u).Error
	encoded := u.PasswordHash
	if e != nil || encoded == "" {
		encoded = b.dummyHash
	}
	valid := passwordOK(encoded, in.Password)
	if e != nil || !valid || !u.Enabled || !u.LocalEnabled {
		b.event("", "", "login", requestID(r), false)
		fail(w, r, 401, "invalid_credentials")
		return
	}
	if in.RequestID != "" {
		a, e := b.browserAuth(r, in.RequestID)
		if e != nil {
			fail(w, r, 400, "invalid_transaction")
			return
		}
		var app Application
		if b.DB.First(&app, "id = ?", a.ClientID).Error != nil || !app.Enabled || !app.LocalEnabled {
			fail(w, r, 403, "authentication_method_denied")
			return
		}
	}
	if e = b.newSession(w, r, u, "password", ""); e != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	b.event(u.ID, u.ID, "login", requestID(r), true)
	out, err := b.meResult(u)
	if err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	if in.RequestID != "" {
		out["redirect"] = "/oidc/login?requestId=" + url.QueryEscape(in.RequestID)
	}
	write(w, 200, out)
}
func requestID(r *http.Request) string { v, _ := r.Context().Value(requestKey).(string); return v }
func (b *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("burrow_session"); e == nil {
		if err := b.DB.Model(&Session{}).Where("credential_hash = ?", hash(c.Value)).Update("revoked", true).Error; err != nil {
			fail(w, r, 503, "unavailable")
			return
		}
	}
	b.cookie(w, "burrow_session", "", -1)
	write(w, 200, map[string]bool{"ok": true})
}
func (b *Server) profile(w http.ResponseWriter, r *http.Request) {
	u, _, ok := b.require(w, r, "")
	if !ok {
		return
	}
	if u.MustChangePassword {
		fail(w, r, 403, "password_change_required")
		return
	}
	var in struct{ Name, Language, Theme *string }
	if !decode(w, r, &in) {
		return
	}
	if (in.Language != nil && !contains([]string{"en", "zh-CN"}, *in.Language)) || (in.Theme != nil && !contains([]string{"light", "dark", "system"}, *in.Theme)) || (in.Name != nil && len(*in.Name) > 200) {
		fail(w, r, 400, "invalid_profile")
		return
	}
	updates := map[string]any{}
	if in.Name != nil {
		updates["name"] = *in.Name
	}
	if in.Language != nil {
		updates["language"] = *in.Language
	}
	if in.Theme != nil {
		updates["theme"] = *in.Theme
	}
	err := b.authorizationTx(r, "", func(tx *gorm.DB, current User, _ Session) error {
		if current.MustChangePassword {
			return errors.New("forbidden")
		}
		if len(updates) > 0 {
			if e := tx.Model(&User{}).Where("id = ?", current.ID).Updates(updates).Error; e != nil {
				return e
			}
		}
		return tx.First(&u, "id = ?", current.ID).Error
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	out, err := b.meResult(u)
	if err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	write(w, 200, out)
}
func (b *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, _, ok := b.require(w, r, "")
	if !ok {
		return
	}
	var in struct{ CurrentPassword, Password, RequestID string }
	if !decode(w, r, &in) {
		return
	}
	if !u.LocalEnabled || !passwordOK(u.PasswordHash, in.CurrentPassword) {
		fail(w, r, 400, "invalid_credentials")
		return
	}
	h, e := passwordHash(in.Password)
	if e != nil {
		fail(w, r, 400, "password_policy")
		return
	}
	e = b.authorizationTx(r, "", func(tx *gorm.DB, current User, session Session) error {
		if !current.LocalEnabled || current.PasswordHash != u.PasswordHash {
			return errors.New("invalid_credentials")
		}
		if err := tx.Model(&User{}).Where("id = ? AND password_hash = ?", current.ID, u.PasswordHash).Updates(map[string]any{"password_hash": h, "must_change_password": false}).Error; err != nil {
			return err
		}
		return tx.Model(&Session{}).Where("user_id = ? AND id <> ?", current.ID, session.ID).Update("revoked", true).Error
	})
	if e != nil {
		mutationError(w, r, e)
		return
	}
	u.MustChangePassword = false
	out, err := b.meResult(u)
	if err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	if in.RequestID != "" {
		out["redirect"] = "/oidc/login?requestId=" + url.QueryEscape(in.RequestID)
	}
	write(w, 200, out)
}
func (b *Server) myApps(w http.ResponseWriter, r *http.Request) {
	u, _, ok := b.require(w, r, "")
	if !ok {
		return
	}
	items := []Application{}
	if !u.MustChangePassword {
		p, admin, e := permissions(b.DB, u)
		if e != nil {
			fail(w, r, 503, "unavailable")
			return
		}
		var all []Application
		if b.DB.Where("enabled = ?", true).Find(&all).Error != nil {
			fail(w, r, 503, "unavailable")
			return
		}
		for _, a := range all {
			if admin || contains(p, "app:"+a.ID+":login") {
				items = append(items, a)
			}
		}
	}
	write(w, 200, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": len(items)})
}
func (b *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	_, _, ok := b.require(w, r, "dashboard:read")
	if !ok {
		return
	}
	out := map[string]int64{}
	for key, m := range map[string]any{"users": &User{}, "applications": &Application{}} {
		var n int64
		if b.DB.Model(m).Count(&n).Error != nil {
			fail(w, r, 503, "unavailable")
			return
		}
		out[key] = n
	}
	for key, success := range map[string]bool{"loginSuccess": true, "loginFailure": false} {
		var n int64
		b.DB.Model(&Event{}).Where("kind = ? AND success = ? AND created_at > ?", "login", success, time.Now().Add(-24*time.Hour)).Count(&n)
		out[key] = n
	}
	write(w, 200, out)
}
