package burrow

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestUpstreamOIDCBrowserNonceAndLinking(t *testing.T) {
	b, _, admin := testServer(t, "sqlite")
	b.Config.AllowPrivateProviders = true
	private, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	nonce, subject := "", "linked-sub"
	badNonce := false
	authTime := time.Now().Unix()
	var issuer *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"issuer": issuer.URL, "authorization_endpoint": issuer.URL + "/authorize", "token_endpoint": issuer.URL + "/token", "jwks_uri": issuer.URL + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &private.PublicKey, KeyID: "upstream", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		client, secret, ok := r.BasicAuth()
		if !ok || client != "burrow-rp" || secret != "rp-secret" || r.Form.Get("code_verifier") == "" {
			t.Error("upstream client authentication / PKCE absent")
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		n := nonce
		if badNonce {
			n = "incorrect"
		}
		claims := map[string]any{"iss": issuer.URL, "aud": "burrow-rp", "sub": subject, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": n, "email": admin.Email, "auth_time": authTime}
		payload, _ := json.Marshal(claims)
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: &jose.JSONWebKey{Key: private, KeyID: "upstream"}}, nil)
		signed, _ := signer.Sign(payload)
		token, _ := signed.CompactSerialize()
		write(w, 200, map[string]any{"access_token": "upstream-access", "token_type": "Bearer", "expires_in": 60, "id_token": token})
	})
	issuer = httptest.NewServer(mux)
	defer issuer.Close()
	secret, _ := b.seal("rp-secret")
	p := Provider{ID: random(18), Name: "upstream", Issuer: issuer.URL, ClientID: "burrow-rp", SecretCipher: secret, Enabled: true}
	if e = b.DB.Create(&p).Error; e != nil {
		t.Fatal(e)
	}
	identity := ExternalIdentity{ID: random(18), ProviderID: p.ID, Issuer: p.Issuer, Subject: subject, UserID: admin.ID}
	if e = b.DB.Create(&identity).Error; e != nil {
		t.Fatal(e)
	}
	start := func(c *browser) string {
		t.Helper()
		w := c.request("GET", "/api/v1/auth/providers/"+p.ID+"/login", nil, false)
		if w.Code != 302 {
			t.Fatalf("start upstream %d %s", w.Code, w.Body.String())
		}
		u, _ := url.Parse(w.Header().Get("Location"))
		nonce = u.Query().Get("nonce")
		if u.Query().Get("prompt") != "login" || u.Query().Get("max_age") != "0" {
			t.Fatal("upstream login must require fresh authentication")
		}
		if nonce == "" || u.Query().Get("code_challenge_method") != "S256" {
			t.Fatal("upstream nonce / PKCE absent")
		}
		return u.Query().Get("state")
	}
	callback := func(c *browser, state string) *httptest.ResponseRecorder {
		return c.request("GET", "/api/v1/auth/providers/"+p.ID+"/callback?code=valid&state="+url.QueryEscape(state), nil, false)
	}
	c := newBrowser(b)
	state := start(c)
	if w := callback(newBrowser(b), state); w.Code != 400 {
		t.Fatal("state accepted from wrong browser")
	}
	if w := callback(c, "wrong-state"); w.Code != 400 {
		t.Fatal("invalid upstream state accepted")
	}
	badNonce = true
	if w := callback(c, state); w.Code != 401 {
		t.Fatalf("invalid nonce accepted %d %s", w.Code, w.Body.String())
	}
	badNonce = false
	if w := callback(c, state); w.Code != 400 {
		t.Fatal("consumed state accepted")
	}
	subject = "same-email-but-unlinked"
	state = start(c)
	if w := callback(c, state); w.Code != 401 || !strings.Contains(w.Body.String(), "external_identity_not_linked") {
		t.Fatalf("unlinked subject accepted %d %s", w.Code, w.Body.String())
	}
	subject = identity.Subject
	authTime = time.Now().Add(-time.Hour).Unix()
	state = start(c)
	if w := callback(c, state); w.Code != 401 {
		t.Fatal("stale upstream authentication accepted")
	}
	authTime = 0
	state = start(c)
	if w := callback(c, state); w.Code != 401 {
		t.Fatal("missing upstream authentication time accepted")
	}
	authTime = time.Now().Unix()
	state = start(c)
	if w := callback(c, state); w.Code != 302 {
		t.Fatalf("linked upstream failed %d %s", w.Code, w.Body.String())
	}
	u, session, e := b.session(cookieRequest(c))
	if e != nil || u.ID != admin.ID || session.Method != "oidc" || session.ProviderID != p.ID {
		t.Fatalf("wrong upstream session %v %+v", e, session)
	}
	a := testApp(t, b, "spa")
	b.DB.Create(&ApplicationProvider{ApplicationID: a.ID, ProviderID: p.ID})
	code, _ := authorize(t, c, a, nil)
	if strings.HasPrefix(code, "/") || strings.HasPrefix(code, "error:") {
		t.Fatal("allowed source session rejected")
	}
	b.DB.Model(&Provider{}).Where("id = ?", p.ID).Update("enabled", false)
	code, _ = authorize(t, c, a, url.Values{"prompt": {"none"}})
	if code != "error:login_required" {
		t.Fatalf("disabled provider source accepted %s", code)
	}
}
func cookieRequest(c *browser) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	for _, cookie := range c.cookies {
		r.AddCookie(cookie)
	}
	return r
}
func TestProviderNetworkPolicy(t *testing.T) {
	b, _, _ := testServer(t, "sqlite")
	for _, raw := range []string{"http://127.0.0.1", "https://127.0.0.1", "https://169.254.169.254", "https://localhost", "https://user:secret@example.com"} {
		if b.validateProviderURL(raw) == nil {
			t.Fatalf("unsafe provider accepted: %s", raw)
		}
	}
	if b.validateProviderURL("https://accounts.example") != nil {
		t.Fatal("HTTPS public issuer rejected")
	}
	for _, raw := range []string{"http://127.0.0.1", "https://127.0.0.1"} {
		r, e := b.providerHTTP().Get(raw)
		if e == nil {
			r.Body.Close()
			t.Fatalf("unsafe provider dial accepted: %s", raw)
		}
	}
}
