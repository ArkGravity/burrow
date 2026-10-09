package burrow

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"gorm.io/gorm"
)

type oidcStore struct{ *Server }
type client struct {
	Application
	development bool
	lifetime    time.Duration
}

func (c client) GetID() string                    { return c.ClientID }
func (c client) RedirectURIs() []string           { return c.RedirectURLs }
func (c client) PostLogoutRedirectURIs() []string { return c.LogoutURLs }
func (c client) ApplicationType() op.ApplicationType {
	if c.ClientType == "spa" {
		return op.ApplicationTypeUserAgent
	}
	return op.ApplicationTypeWeb
}
func (c client) AuthMethod() oidc.AuthMethod {
	if c.ClientType == "spa" {
		return oidc.AuthMethodNone
	}
	return oidc.AuthMethodBasic
}
func (c client) ResponseTypes() []oidc.ResponseType {
	return []oidc.ResponseType{oidc.ResponseTypeCode}
}
func (c client) GrantTypes() []oidc.GrantType        { return []oidc.GrantType{oidc.GrantTypeCode} }
func (c client) LoginURL(id string) string           { return "/oidc/login?requestId=" + url.QueryEscape(id) }
func (c client) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeBearer }
func (c client) IDTokenLifetime() time.Duration      { return c.lifetime }
func (c client) DevMode() bool                       { return c.development }
func (c client) RestrictAdditionalIdTokenScopes() func([]string) []string {
	return func(scopes []string) []string { return scopes }
}
func (c client) RestrictAdditionalAccessTokenScopes() func([]string) []string {
	return func(scopes []string) []string { return scopes }
}
func (c client) IsScopeAllowed(s string) bool {
	return contains([]string{"openid", "profile", "email"}, s)
}
func (c client) IDTokenUserinfoClaimsAssertion() bool { return true }
func (c client) ClockSkew() time.Duration             { return 0 }

type authRequest struct {
	AuthTransaction
	Request oidc.AuthRequest
	MFAAt   time.Time
}

func (a *authRequest) GetID() string  { return a.ID }
func (a *authRequest) GetACR() string { return "" }
func (a *authRequest) GetAMR() []string {
	if !a.MFAAt.IsZero() {
		return []string{"pwd", "otp"}
	}
	return []string{"pwd"}
}
func (a *authRequest) GetAudience() []string  { return []string{a.Request.ClientID} }
func (a *authRequest) GetAuthTime() time.Time { return a.AuthTime }
func (a *authRequest) GetClientID() string    { return a.Request.ClientID }
func (a *authRequest) GetCodeChallenge() *oidc.CodeChallenge {
	if a.Request.CodeChallenge == "" {
		return nil
	}
	return &oidc.CodeChallenge{Challenge: a.Request.CodeChallenge, Method: a.Request.CodeChallengeMethod}
}
func (a *authRequest) GetNonce() string                   { return a.Request.Nonce }
func (a *authRequest) GetRedirectURI() string             { return a.Request.RedirectURI }
func (a *authRequest) GetResponseType() oidc.ResponseType { return oidc.ResponseTypeCode }
func (a *authRequest) GetResponseMode() oidc.ResponseMode { return a.Request.ResponseMode }
func (a *authRequest) GetScopes() []string                { return a.Request.Scopes }
func (a *authRequest) GetState() string                   { return a.Request.State }
func (a *authRequest) GetSubject() string                 { return a.UserID }
func (a *authRequest) Done() bool                         { return a.UserID != "" && a.SessionID != "" }
func loadAuth(v AuthTransaction) (*authRequest, error) {
	a := &authRequest{AuthTransaction: v}
	e := json.Unmarshal([]byte(v.Payload), &a.Request)
	return a, e
}
func (s oidcStore) CreateAuthRequest(ctx context.Context, r *oidc.AuthRequest, _ string) (op.AuthRequest, error) {
	for _, scope := range r.Scopes {
		if !contains([]string{"openid", "profile", "email"}, scope) {
			return nil, oidc.ErrInvalidScope()
		}
	}
	if !contains(r.Scopes, "openid") {
		return nil, oidc.ErrInvalidScope()
	}
	if r.ResponseMode != "" && r.ResponseMode != oidc.ResponseModeQuery {
		return nil, oidc.ErrInvalidRequest()
	}
	req, _ := ctx.Value(contextKey("httpRequest")).(*http.Request)
	if req == nil {
		return nil, errors.New("request context missing")
	}
	browser, e := req.Cookie("burrow_browser")
	if e != nil {
		return nil, e
	}
	var app Application
	if e = s.DB.Where("client_id = ? AND enabled = ?", r.ClientID, true).First(&app).Error; e != nil {
		return nil, e
	}
	if e = validatePKCE(app, r); e != nil {
		return nil, e
	}
	payload, e := json.Marshal(r)
	if e != nil {
		return nil, e
	}
	a := AuthTransaction{ID: random(24), Payload: string(payload), BrowserHash: hash(browser.Value), ClientID: app.ID, ExpiresAt: time.Now().Add(s.Config.LoginTTL)}
	e = s.DB.Create(&a).Error
	if e != nil {
		return nil, e
	}
	return loadAuth(a)
}
func (s oidcStore) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	var a AuthTransaction
	if e := s.DB.WithContext(ctx).Where("id = ? AND expires_at > ? AND consumed = ?", id, time.Now(), false).First(&a).Error; e != nil {
		return nil, e
	}
	if a.UserID != "" {
		return s.authenticatedAuth(s.DB, a)
	}
	return loadAuth(a)
}
func (s oidcStore) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	var a AuthTransaction
	if e := s.DB.WithContext(ctx).Where("code_hash = ? AND code_expires_at > ? AND consumed = ?", hash(code), time.Now(), false).First(&a).Error; e != nil {
		return nil, e
	}
	return s.authenticatedAuth(s.DB, a)
}
func (b *Server) authenticatedAuth(tx *gorm.DB, a AuthTransaction) (*authRequest, error) {
	_, session, err := b.authorized(tx, a)
	if err != nil {
		return nil, err
	}
	request, err := loadAuth(a)
	if err == nil {
		request.MFAAt = session.MFAAt
	}
	return request, err
}
func (s oidcStore) SaveAuthCode(ctx context.Context, id, code string) error {
	h := hash(code)
	q := s.DB.WithContext(ctx).Model(&AuthTransaction{}).Where("id = ? AND code_hash IS NULL AND consumed = ? AND expires_at > ?", id, false, time.Now()).Updates(map[string]any{"code_hash": &h, "code_expires_at": time.Now().Add(s.Config.AuthCodeTTL)})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return oidc.ErrInvalidGrant()
	}
	return nil
}
func (s oidcStore) DeleteAuthRequest(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Where("id = ?", id).Delete(&AuthTransaction{}).Error
}
func (b *Server) authorized(tx *gorm.DB, a AuthTransaction) (User, Session, error) {
	var u User
	var session Session
	var app Application
	if e := tx.Where("id = ? AND enabled = ? AND must_change_password = ? AND password_hash <> ?", a.UserID, true, false, "").First(&u).Error; e != nil {
		return u, session, e
	}
	if e := tx.Where("id = ? AND revoked = ? AND expires_at > ? AND user_id = ?", a.SessionID, false, time.Now(), u.ID).First(&session).Error; e != nil {
		return u, session, e
	}
	if e := tx.Where("id = ? AND enabled = ?", a.ClientID, true).First(&app).Error; e != nil {
		return u, session, e
	}
	// Token-backed UserInfo checks have no authorization request payload.
	if a.Payload != "" {
		request, e := loadAuth(a)
		if e != nil {
			return u, session, e
		}
		if e := validatePKCE(app, &request.Request); e != nil {
			return u, session, e
		}
	}
	p, admin, e := permissions(tx, u)
	if e != nil {
		return u, session, e
	}
	if !admin && !contains(p, "app:"+app.ID+":login") {
		return u, session, oidc.ErrAccessDenied()
	}
	if session.Method != "password" || !b.validMFASession(u, session) {
		return u, session, oidc.ErrLoginRequired()
	}
	return u, session, nil
}

func validatePKCE(app Application, request *oidc.AuthRequest) error {
	if request.CodeChallenge == "" && request.CodeChallengeMethod == "" && app.ClientType == "web" && app.AllowWithoutPKCE {
		return nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(request.CodeChallenge)
	if request.CodeChallengeMethod != oidc.CodeChallengeMethodS256 || err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != request.CodeChallenge {
		return oidc.ErrInvalidRequest().WithDescription("S256 PKCE required")
	}
	return nil
}
func (s oidcStore) CreateAccessToken(ctx context.Context, request op.TokenRequest) (string, time.Time, error) {
	a, ok := request.(*authRequest)
	if !ok {
		return "", time.Time{}, oidc.ErrUnsupportedGrantType()
	}
	token := TokenRecord{ID: random(32), UserID: a.UserID, ClientID: a.ClientID, SessionID: a.SessionID, Scopes: a.Request.Scopes, ExpiresAt: time.Now().Add(s.Config.TokenTTL)}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if s.Config.DBDriver == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(734285622)").Error; err != nil {
				return err
			}
		}
		if _, _, e := s.authorized(tx, a.AuthTransaction); e != nil {
			return oidc.ErrInvalidGrant()
		}
		q := tx.Model(&AuthTransaction{}).Where("id = ? AND consumed = ? AND code_expires_at > ?", a.ID, false, time.Now()).Update("consumed", true)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return oidc.ErrInvalidGrant()
		}
		return tx.Create(&token).Error
	})
	return token.ID, token.ExpiresAt, err
}
func (s oidcStore) CreateAccessAndRefreshTokens(context.Context, op.TokenRequest, string) (string, string, time.Time, error) {
	return "", "", time.Time{}, oidc.ErrUnsupportedGrantType()
}
func (s oidcStore) TokenRequestByRefreshToken(context.Context, string) (op.RefreshTokenRequest, error) {
	return nil, op.ErrInvalidRefreshToken
}
func (s oidcStore) GetRefreshTokenInfo(context.Context, string, string) (string, string, error) {
	return "", "", op.ErrInvalidRefreshToken
}
func (s oidcStore) TerminateSession(context.Context, string, string) error {
	return errors.New("use confirmed logout endpoint")
}
func (s oidcStore) RevokeToken(context.Context, string, string, string) *oidc.Error {
	return oidc.ErrUnsupportedGrantType()
}
func (s oidcStore) GetClientByClientID(ctx context.Context, id string) (op.Client, error) {
	var a Application
	e := s.DB.WithContext(ctx).Where("client_id = ? AND enabled = ?", id, true).First(&a).Error
	return client{a, s.Config.Env == "dev", s.Config.TokenTTL}, e
}
func (s oidcStore) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	var a Application
	if e := s.DB.WithContext(ctx).Where("client_id = ? AND enabled = ? AND client_type = ?", id, true, "web").First(&a).Error; e != nil {
		return oidc.ErrInvalidClient()
	}
	if !hmac.Equal([]byte(a.SecretHash), []byte(hash(secret))) {
		return oidc.ErrInvalidClient()
	}
	return nil
}
func fillUserInfo(info *oidc.UserInfo, u User, scopes []string) {
	info.Subject = u.ID
	if contains(scopes, "profile") {
		info.Name = u.Name
		info.PreferredUsername = u.Username
	}
	if contains(scopes, "email") {
		info.Email = u.Email
	}
}
func (s oidcStore) SetUserinfoFromScopes(context.Context, *oidc.UserInfo, string, string, []string) error {
	return nil
}
func (s oidcStore) SetUserinfoFromRequest(ctx context.Context, info *oidc.UserInfo, request op.IDTokenRequest, scopes []string) error {
	var u User
	if e := s.DB.WithContext(ctx).First(&u, "id = ?", request.GetSubject()).Error; e != nil {
		return e
	}
	fillUserInfo(info, u, scopes)
	return nil
}
func (s oidcStore) SetUserinfoFromToken(ctx context.Context, info *oidc.UserInfo, id, subject, origin string) error {
	var token TokenRecord
	if e := s.DB.WithContext(ctx).Where("id = ? AND user_id = ? AND revoked = ? AND expires_at > ?", id, subject, false, time.Now()).First(&token).Error; e != nil {
		return e
	}
	u, _, e := s.authorized(s.DB, AuthTransaction{UserID: token.UserID, ClientID: token.ClientID, SessionID: token.SessionID})
	if e != nil {
		return e
	}
	if origin != "" {
		var a Application
		if e = s.DB.First(&a, "id = ?", token.ClientID).Error; e != nil || !contains(a.Origins, origin) {
			return errors.New("origin denied")
		}
	}
	fillUserInfo(info, u, token.Scopes)
	return nil
}
func (s oidcStore) SetIntrospectionFromToken(context.Context, *oidc.IntrospectionResponse, string, string, string) error {
	return errors.New("unsupported")
}
func (s oidcStore) GetPrivateClaimsFromScopes(context.Context, string, string, []string) (map[string]any, error) {
	return nil, nil
}
func (s oidcStore) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errors.New("unsupported")
}
func (s oidcStore) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, errors.New("unsupported")
}

type key struct {
	id    string
	value any
}

func (k key) ID() string                                  { return k.id }
func (k key) Key() any                                    { return k.value }
func (k key) Use() string                                 { return "sig" }
func (k key) Algorithm() jose.SignatureAlgorithm          { return jose.RS256 }
func (k key) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (s oidcStore) SigningKey(ctx context.Context) (op.SigningKey, error) {
	var k SigningKey
	if e := s.DB.WithContext(ctx).Where("active = ?", true).First(&k).Error; e != nil {
		return nil, e
	}
	raw, e := s.unseal(k.PrivateCipher)
	if e != nil {
		return nil, e
	}
	der, e := base64.RawStdEncoding.DecodeString(raw)
	if e != nil {
		return nil, e
	}
	pk, e := x509.ParsePKCS1PrivateKey(der)
	return key{k.ID, pk}, e
}
func (s oidcStore) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}
func (s oidcStore) KeySet(ctx context.Context) ([]op.Key, error) {
	var keys []SigningKey
	if e := s.DB.WithContext(ctx).Find(&keys).Error; e != nil {
		return nil, e
	}
	out := []op.Key{}
	for _, k := range keys {
		var jwk jose.JSONWebKey
		if e := json.Unmarshal([]byte(k.PublicJSON), &jwk); e != nil {
			return nil, e
		}
		out = append(out, key{k.ID, jwk.Key})
	}
	return out, nil
}
func (s *Store) RotateKeys() error {
	private, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		return e
	}
	id := random(18)
	pub, e := json.Marshal(jose.JSONWebKey{Key: &private.PublicKey, KeyID: id, Algorithm: "RS256", Use: "sig"})
	if e != nil {
		return e
	}
	encrypted, e := s.seal(base64.RawStdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(private)))
	if e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if s.Config.DBDriver == "postgres" {
			if e := tx.Exec("SELECT pg_advisory_xact_lock(734285623)").Error; e != nil {
				return e
			}
		}
		if e := tx.Model(&SigningKey{}).Where("active = ?", true).Update("active", false).Error; e != nil {
			return e
		}
		return tx.Create(&SigningKey{ID: id, PrivateCipher: encrypted, PublicJSON: string(pub), Active: true, CreatedAt: time.Now()}).Error
	})
}

var _ op.Storage = oidcStore{}
