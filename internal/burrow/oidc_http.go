package burrow

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

func (b *Server) initOIDC() error {
	opts := []op.Option{op.WithCustomAuthEndpoint(op.NewEndpoint("/oidc/authorize")), op.WithCustomTokenEndpoint(op.NewEndpoint("/oidc/token")), op.WithCustomUserinfoEndpoint(op.NewEndpoint("/oidc/userinfo")), op.WithCustomKeysEndpoint(op.NewEndpoint("/oidc/jwks")), op.WithCustomEndSessionEndpoint(op.NewEndpoint("/oidc/logout")), op.WithCORSOptions(nil)}
	if b.Config.Env == "dev" {
		opts = append(opts, op.WithAllowInsecure())
	}
	p, e := op.NewProvider(&op.Config{CryptoKey: b.Config.MasterKey, CodeMethodS256: true, DefaultLogoutRedirectURI: b.Config.Issuer + "/login", SupportedScopes: []string{"openid", "profile", "email"}}, oidcStore{b}, op.StaticIssuer(b.Config.Issuer), opts...)
	b.OP = p
	return e
}
func (b *Server) discovery(w http.ResponseWriter, r *http.Request) {
	if !b.readCORS(w, r) {
		return
	}
	i := b.Config.Issuer
	write(w, 200, map[string]any{"issuer": i, "authorization_endpoint": i + "/oidc/authorize", "token_endpoint": i + "/oidc/token", "userinfo_endpoint": i + "/oidc/userinfo", "jwks_uri": i + "/oidc/jwks", "end_session_endpoint": i + "/oidc/logout", "scopes_supported": []string{"openid", "profile", "email"}, "response_types_supported": []string{"code"}, "response_modes_supported": []string{"query"}, "grant_types_supported": []string{"authorization_code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "token_endpoint_auth_methods_supported": []string{"client_secret_basic", "none"}, "code_challenge_methods_supported": []string{"S256"}, "claims_supported": []string{"sub", "iss", "aud", "exp", "iat", "auth_time", "nonce", "at_hash", "name", "preferred_username", "email"}, "request_parameter_supported": false, "request_uri_parameter_supported": false})
}
func (b *Server) protocol(w http.ResponseWriter, r *http.Request) {
	if (r.URL.Path == "/oidc/jwks" || r.URL.Path == "/oidc/userinfo") && !b.readCORS(w, r) {
		return
	}
	if r.URL.Path == "/oidc/token" {
		if b.limited(r, "token") {
			write(w, 429, map[string]string{"error": "temporarily_unavailable"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if e := r.ParseForm(); e != nil {
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			var apps []Application
			if b.DB.Where("enabled = ? AND client_type = ?", true, "spa").Find(&apps).Error != nil {
				write(w, 503, map[string]string{"error": "server_error"})
				return
			}
			allowed := false
			for _, a := range apps {
				if contains(a.Origins, origin) && (r.Method == "OPTIONS" || a.ClientID == r.Form.Get("client_id")) {
					allowed = true
				}
			}
			if !allowed {
				write(w, 403, map[string]string{"error": "invalid_request"})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			write(w, 400, map[string]string{"error": "unsupported_grant_type"})
			return
		}
		clientID, _, basic := r.BasicAuth()
		if basic {
			clientID, _ = url.QueryUnescape(clientID)
		} else {
			clientID = r.Form.Get("client_id")
		}
		var app Application
		if b.DB.Where("client_id = ? AND enabled = ?", clientID, true).First(&app).Error != nil || (app.ClientType == "web" && !basic) || (app.ClientType == "spa" && (basic || r.Form.Get("client_secret") != "")) {
			write(w, 401, map[string]string{"error": "invalid_client"})
			return
		}
		if r.Form.Get("client_secret") != "" || r.Form.Get("client_assertion") != "" {
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
	}
	if r.URL.Path == "/oidc/authorize" {
		v := b.browser(w, r)
		r.AddCookie(&http.Cookie{Name: "burrow_browser", Value: v})
		q := r.URL.Query()
		var app Application
		if b.DB.Where("client_id = ? AND enabled = ?", q.Get("client_id"), true).First(&app).Error != nil || !contains(app.RedirectURLs, q.Get("redirect_uri")) {
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		for _, p := range []string{"request_uri", "request"} {
			if q.Get(p) != "" {
				write(w, 400, map[string]string{"error": "request_not_supported"})
				return
			}
		}
	}
	if r.URL.Path == "/oidc/authorize/callback" {
		if _, e := b.browserAuth(r, r.URL.Query().Get("id")); e != nil {
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
	}
	r = r.WithContext(context.WithValue(r.Context(), contextKey("httpRequest"), r))
	b.OP.ServeHTTP(w, r)
}
func (b *Server) browserAuth(r *http.Request, id string) (*authRequest, error) {
	var a AuthTransaction
	cookie, e := r.Cookie("burrow_browser")
	if e != nil {
		return nil, e
	}
	if e = b.DB.Where("id = ? AND browser_hash = ? AND expires_at > ? AND consumed = ?", id, hash(cookie.Value), time.Now(), false).First(&a).Error; e != nil {
		return nil, e
	}
	return loadAuth(a)
}
func (b *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("requestId")
	a, e := b.browserAuth(r, id)
	if e != nil {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	u, session, e := b.session(r)
	mustLogin := e != nil
	if !mustLogin {
		if a.Request.MaxAge != nil {
			max := time.Duration(*a.Request.MaxAge) * time.Second
			if *a.Request.MaxAge == 0 {
				mustLogin = session.AuthTime.Unix() < a.ExpiresAt.Add(-b.Config.LoginTTL).Unix()
			} else {
				mustLogin = time.Since(session.AuthTime) > max
			}
		}
		a.UserID = u.ID
		a.SessionID = session.ID
		a.AuthTime = session.AuthTime
		a.Method = session.Method
		if _, _, e = b.authorized(b.DB, a.AuthTransaction); e != nil {
			if errors.Is(e, oidc.ErrAccessDenied()) {
				b.authError(w, r, a, "access_denied")
				return
			}
			mustLogin = true
		}
	}
	if mustLogin {
		if contains(a.Request.Prompt, "none") {
			b.authError(w, r, a, "login_required")
			return
		}
		if u.MustChangePassword {
			http.Redirect(w, r, "/change-password?requestId="+url.QueryEscape(id), 302)
			return
		}
		http.Redirect(w, r, "/login?requestId="+url.QueryEscape(id), 302)
		return
	}
	if a.Request.IDTokenHint != "" {
		ctx := op.ContextWithIssuer(r.Context(), b.Config.Issuer)
		claims, e := op.VerifyIDTokenHint[*oidc.TokenClaims](ctx, a.Request.IDTokenHint, b.OP.IDTokenHintVerifier(ctx))
		if e != nil || claims.GetSubject() != u.ID {
			b.authError(w, r, a, "login_required")
			return
		}
	}
	if e = b.DB.Model(&AuthTransaction{}).Where("id = ?", id).Updates(map[string]any{"user_id": u.ID, "session_id": session.ID, "auth_time": session.AuthTime, "method": session.Method}).Error; e != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	http.Redirect(w, r, "/oidc/authorize/callback?id="+url.QueryEscape(id), 302)
}
func (b *Server) authError(w http.ResponseWriter, r *http.Request, a *authRequest, code string) {
	u, e := url.Parse(a.Request.RedirectURI)
	if e != nil {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	q := u.Query()
	q.Set("error", code)
	if a.Request.State != "" {
		q.Set("state", a.Request.State)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), 302)
}
func (b *Server) authContext(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("requestId")
	name := ""
	if id != "" {
		a, e := b.browserAuth(r, id)
		if e != nil {
			fail(w, r, 400, "invalid_transaction")
			return
		}
		var app Application
		if b.DB.First(&app, "id = ? AND enabled = ?", a.ClientID, true).Error != nil {
			fail(w, r, 400, "invalid_application")
			return
		}
		name = app.Name
	}
	write(w, 200, map[string]string{"applicationName": name})
}
func (b *Server) oidcLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		target := "/logout"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, 302)
		return
	}
	if !b.validCSRF(r) {
		fail(w, r, 403, "csrf_invalid")
		return
	}
	var in struct {
		IDTokenHint           string `json:"idTokenHint"`
		PostLogoutRedirectURI string `json:"postLogoutRedirectUri"`
		State                 string `json:"state"`
	}
	if !decode(w, r, &in) {
		return
	}
	redirect := "/login"
	if in.PostLogoutRedirectURI != "" || in.IDTokenHint != "" {
		if in.IDTokenHint == "" {
			fail(w, r, 400, "invalid_logout")
			return
		}
		ctx := op.ContextWithIssuer(r.Context(), b.Config.Issuer)
		claims, e := op.VerifyIDTokenHint[*oidc.TokenClaims](ctx, in.IDTokenHint, b.OP.IDTokenHintVerifier(ctx))
		if e != nil {
			fail(w, r, 400, "invalid_logout")
			return
		}
		u, _, e := b.session(r)
		if e != nil || claims.GetSubject() != u.ID {
			fail(w, r, 400, "invalid_logout")
			return
		}
		var app Application
		aud := claims.GetAudience()
		if len(aud) != 1 || b.DB.Where("client_id = ?", aud[0]).First(&app).Error != nil {
			fail(w, r, 400, "invalid_logout")
			return
		}
		if in.PostLogoutRedirectURI != "" {
			if !contains(app.LogoutURLs, in.PostLogoutRedirectURI) {
				fail(w, r, 400, "invalid_logout")
				return
			}
			redirect = in.PostLogoutRedirectURI
			if in.State != "" {
				u, _ := url.Parse(redirect)
				q := u.Query()
				q.Set("state", in.State)
				u.RawQuery = q.Encode()
				redirect = u.String()
			}
		}
	}
	if c, e := r.Cookie("burrow_session"); e == nil {
		if e = b.DB.Model(&Session{}).Where("credential_hash = ?", hash(c.Value)).Update("revoked", true).Error; e != nil {
			fail(w, r, 503, "unavailable")
			return
		}
	}
	b.cookie(w, "burrow_session", "", -1)
	write(w, 200, map[string]string{"redirect": redirect})
}
