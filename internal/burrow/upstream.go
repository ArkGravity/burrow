package burrow

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

func (b *Server) validateProviderURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "https" && !(b.Config.AllowPrivateProviders && b.Config.Env == "dev" && u.Scheme == "http")) {
		return invalid("provider_url")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !b.providerAddressAllowed(ip) {
		return invalid("provider_url")
	}
	if u.Hostname() == "localhost" && !b.Config.AllowPrivateProviders {
		return invalid("provider_url")
	}
	return nil
}
func restrictedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

type safeTransport struct {
	base      *http.Transport
	allowHTTP bool
}

func (t safeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" && !(t.allowHTTP && r.URL.Scheme == "http") {
		return nil, errors.New("provider requires HTTPS")
	}
	if r.URL.User != nil {
		return nil, errors.New("userinfo in provider URL forbidden")
	}
	return t.base.RoundTrip(r)
}
func (b *Server) providerHTTP() *http.Client {
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("provider host unresolved")
		}
		for _, ip := range ips {
			if !b.providerAddressAllowed(ip.IP) {
				return nil, errors.New("private provider address forbidden")
			}
		}
		d := net.Dialer{Timeout: 5 * time.Second}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}}
	return &http.Client{Timeout: 10 * time.Second, Transport: safeTransport{transport, b.Config.AllowPrivateProviders && b.Config.Env == "dev"}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("provider redirects forbidden") }}
}
func (b *Server) relyingParty(ctx context.Context, p Provider, nonce string) (rp.RelyingParty, error) {
	secret, e := b.unseal(p.SecretCipher)
	if e != nil {
		return nil, e
	}
	return rp.NewRelyingPartyOIDC(ctx, p.Issuer, p.ClientID, secret, b.Config.Issuer+"/api/v1/auth/providers/"+p.ID+"/callback", []string{"openid", "profile", "email"}, rp.WithHTTPClient(b.providerHTTP()), rp.WithAuthStyle(oauth2.AuthStyleInHeader), rp.WithVerifierOpts(rp.WithNonce(func(context.Context) string { return nonce })))
}
func (b *Server) providerAllowed(r *http.Request, pid, requestID string) error {
	if requestID == "" {
		return nil
	}
	a, e := b.browserAuth(r, requestID)
	if e != nil {
		return e
	}
	var n int64
	if e = b.DB.Model(&ApplicationProvider{}).Where("application_id = ? AND provider_id = ? AND application_id IN (SELECT id FROM applications WHERE enabled = ?)", a.ClientID, pid, true).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return errors.New("provider denied")
	}
	return nil
}
func (b *Server) upstreamLogin(w http.ResponseWriter, r *http.Request) {
	if b.limited(r, "upstream") {
		fail(w, r, 429, "rate_limited")
		return
	}
	id := chi.URLParam(r, "id")
	request := r.URL.Query().Get("requestId")
	var p Provider
	if b.DB.Where("id = ? AND enabled = ?", id, true).First(&p).Error != nil || b.providerAllowed(r, id, request) != nil {
		fail(w, r, 400, "invalid_provider")
		return
	}
	browser := b.browser(w, r)
	t := UpstreamTransaction{ID: random(32), ProviderID: id, RequestID: request, BrowserHash: hash(browser), Nonce: random(32), Verifier: random(32), ExpiresAt: time.Now().Add(b.Config.LoginTTL)}
	party, e := b.relyingParty(r.Context(), p, t.Nonce)
	if e != nil {
		fail(w, r, 502, "provider_unavailable")
		return
	}
	if e = b.validateProviderURL(party.OAuthConfig().Endpoint.AuthURL); e != nil {
		fail(w, r, 502, "provider_unavailable")
		return
	}
	if e = b.DB.Create(&t).Error; e != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	target := party.OAuthConfig().AuthCodeURL(t.ID, oauth2.SetAuthURLParam("nonce", t.Nonce), oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"), oauth2.S256ChallengeOption(t.Verifier))
	http.Redirect(w, r, target, 302)
}
func (b *Server) upstreamCallback(w http.ResponseWriter, r *http.Request) {
	var t UpstreamTransaction
	browser, e := r.Cookie("burrow_browser")
	if e != nil {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	id := chi.URLParam(r, "id")
	if e = b.DB.Where("id = ? AND provider_id = ? AND browser_hash = ? AND consumed = ? AND expires_at > ?", r.URL.Query().Get("state"), id, hash(browser.Value), false, time.Now()).First(&t).Error; e != nil {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	q := b.DB.Model(&UpstreamTransaction{}).Where("id = ? AND consumed = ?", t.ID, false).Update("consumed", true)
	if q.Error != nil || q.RowsAffected != 1 {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	var p Provider
	if b.DB.Where("id = ? AND enabled = ?", id, true).First(&p).Error != nil || b.providerAllowed(r, id, t.RequestID) != nil {
		fail(w, r, 400, "invalid_provider")
		return
	}
	party, e := b.relyingParty(r.Context(), p, t.Nonce)
	if e != nil {
		fail(w, r, 502, "provider_unavailable")
		return
	}
	tokens, e := rp.CodeExchange[*oidc.IDTokenClaims](r.Context(), r.URL.Query().Get("code"), party, func() []oauth2.AuthCodeOption { return []oauth2.AuthCodeOption{oauth2.VerifierOption(t.Verifier)} })
	if e != nil {
		b.event("", "", "login", requestID(r), false)
		fail(w, r, 401, "upstream_auth_failed")
		return
	}
	// Every interactive upstream login requests fresh authentication; existing
	// Burrow sessions are reused before this flow. max_age requires auth_time.
	authTime := tokens.IDTokenClaims.GetAuthTime()
	started := t.ExpiresAt.Add(-b.Config.LoginTTL)
	if authTime.IsZero() || authTime.Unix() < started.Unix() || authTime.After(time.Now().Add(time.Minute)) {
		fail(w, r, 401, "upstream_reauthentication_required")
		return
	}
	var identity ExternalIdentity
	if b.DB.Where("provider_id = ? AND issuer = ? AND subject = ?", id, p.Issuer, tokens.IDTokenClaims.GetSubject()).First(&identity).Error != nil {
		b.event("", "", "login", requestID(r), false)
		fail(w, r, 401, "external_identity_not_linked")
		return
	}
	var u User
	if b.DB.Where("id = ? AND enabled = ?", identity.UserID, true).First(&u).Error != nil {
		fail(w, r, 401, "external_identity_not_linked")
		return
	}
	if e = b.newSessionAt(w, r, u, "oidc", id, authTime, tokens.IDTokenClaims.GetSubject()); e != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	b.event(u.ID, u.ID, "login", requestID(r), true)
	target := "/"
	if t.RequestID != "" {
		target = "/oidc/login?requestId=" + url.QueryEscape(t.RequestID)
	}
	http.Redirect(w, r, target, 302)
}
