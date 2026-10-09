package burrow

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

const restrictedLoginTTL = 5 * time.Minute

// RFC 4226 dynamic truncation, with RFC 6238's 30-second moving factor.
func otpCode(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 || step < 0 {
		return "", errors.New("invalid_mfa_secret")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}
func otpStep(secret, code string, now time.Time, last int64) (int64, bool) {
	if len(code) != 6 || strings.IndexFunc(code, func(c rune) bool { return c < '0' || c > '9' }) >= 0 {
		return 0, false
	}
	current := now.Unix() / 30
	for _, step := range []int64{current, current - 1, current + 1} {
		expected, err := otpCode(secret, step)
		if err == nil && step > last && subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
func newMFASecret() (string, error) {
	key := make([]byte, 20)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key), nil
}
func (b *Server) validMFASession(u User, s Session) bool {
	return !u.MustChangePassword && s.AuthVersion == u.AuthVersion &&
		(!b.Config.MFAEnabled || (u.MFAEnabled && u.MFACipher != "" && !s.MFAAt.IsZero()))
}

// Share the administrative lock with password, account and MFA mutations.
// The PostgreSQL lock also serializes separate processes (including the CLI).
func (s *Store) authenticationTx(f func(*gorm.DB) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if s.Config.DBDriver == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(734285622)").Error; err != nil {
				return err
			}
		}
		return f(tx)
	})
}
func (b *Server) loginStep(u User, l LoginTransaction) string {
	if b.Config.MFAEnabled && u.MFAEnabled && !l.MFAVerified {
		return "verify"
	}
	if u.MustChangePassword {
		return "password"
	}
	if b.Config.MFAEnabled && !u.MFAEnabled {
		return "bind"
	}
	return "complete"
}
func (b *Server) loginStateDB(tx *gorm.DB, r *http.Request) (User, LoginTransaction, error) {
	var u User
	var l LoginTransaction
	credential, err := r.Cookie("burrow_login")
	browser, e := r.Cookie("burrow_browser")
	if err != nil || e != nil {
		return u, l, errors.New("invalid_transaction")
	}
	if err = tx.Where("credential_hash = ? AND browser_hash = ? AND expires_at > ? AND attempts < ?", hash(credential.Value), hash(browser.Value), time.Now(), 5).First(&l).Error; err != nil {
		return u, l, errors.New("invalid_transaction")
	}
	if err = tx.Where("id = ? AND enabled = ? AND auth_version = ? AND password_hash <> ?", l.UserID, true, l.AuthVersion, "").First(&u).Error; err != nil {
		return u, l, errors.New("invalid_transaction")
	}
	if err := b.validateLoginRequest(tx, u, l); err != nil {
		return u, l, err
	}
	return u, l, nil
}
func (b *Server) validateLoginRequest(tx *gorm.DB, u User, l LoginTransaction) error {
	if l.RequestID != "" {
		var a AuthTransaction
		if tx.Where("id = ? AND browser_hash = ? AND expires_at > ? AND consumed = ?", l.RequestID, l.BrowserHash, time.Now(), false).First(&a).Error != nil {
			return errors.New("invalid_transaction")
		}
		var app Application
		if tx.Where("id = ? AND enabled = ?", a.ClientID, true).First(&app).Error != nil {
			return errors.New("invalid_transaction")
		}
		// Recheck the application's current permission before completing authentication.
		p, admin, err := permissions(tx, u)
		if err != nil {
			return err
		}
		if !admin && !contains(p, "app:"+app.ID+":login") {
			return errors.New("forbidden")
		}
	}
	return nil
}
func (b *Server) loginResult(u User, l LoginTransaction) map[string]any {
	return map[string]any{"step": b.loginStep(u, l), "expiresAt": l.ExpiresAt, "requestId": l.RequestID}
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
	if b.limitedKey("login-account:"+hash(in.Username), 30) {
		fail(w, r, 429, "rate_limited")
		return
	}
	var u User
	err := b.DB.Where("username = ?", in.Username).First(&u).Error
	encoded := u.PasswordHash
	if err != nil || encoded == "" {
		encoded = b.dummyHash
	}
	valid := passwordOK(encoded, in.Password)
	if err != nil || !valid || !u.Enabled {
		b.event("", "", "login", requestID(r), false)
		fail(w, r, 401, "invalid_credentials")
		return
	}
	browser, err := r.Cookie("burrow_browser")
	if err != nil {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	credential := random(32)
	l := LoginTransaction{ID: random(18), CredentialHash: hash(credential), BrowserHash: hash(browser.Value), UserID: u.ID, AuthVersion: u.AuthVersion, RequestID: in.RequestID, ExpiresAt: time.Now().Add(restrictedLoginTTL)}
	var out map[string]any
	var sessionCredential string
	err = b.authenticationTx(func(tx *gorm.DB) error {
		var current User
		if tx.Where("id = ? AND enabled = ?", u.ID, true).First(&current).Error != nil || current.PasswordHash != u.PasswordHash || current.AuthVersion != u.AuthVersion {
			return errors.New("invalid_credentials")
		}
		if err := b.validateLoginRequest(tx, current, l); err != nil {
			return err
		}
		if c, e := r.Cookie("burrow_login"); e == nil {
			if err := tx.Where("credential_hash = ?", hash(c.Value)).Delete(&LoginTransaction{}).Error; err != nil {
				return err
			}
		}
		// Starting fresh authentication removes access through this browser's old session.
		if c, e := r.Cookie("burrow_session"); e == nil {
			if err := tx.Model(&Session{}).Where("credential_hash = ?", hash(c.Value)).Update("revoked", true).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&l).Error; err != nil {
			return err
		}
		out, sessionCredential, err = b.finishLogin(tx, current, l, r)
		return err
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	if sessionCredential == "" {
		b.cookie(w, "burrow_session", "", -1)
		b.cookie(w, "burrow_login", credential, int(restrictedLoginTTL.Seconds()))
	}
	b.loginResponse(w, out, sessionCredential)
}
func (b *Server) loginStatus(w http.ResponseWriter, r *http.Request) {
	u, l, err := b.loginStateDB(b.DB, r)
	if err != nil {
		mutationError(w, r, err)
		return
	}
	// A policy change may remove the remaining MFA step. Require a fresh
	// password login rather than returning "complete" without a shared session.
	if b.loginStep(u, l) == "complete" {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	write(w, 200, b.loginResult(u, l))
}
func (b *Server) loginBind(w http.ResponseWriter, r *http.Request) {
	var secret string
	err := b.authenticationTx(func(tx *gorm.DB) error {
		u, l, err := b.loginStateDB(tx, r)
		if err != nil {
			return err
		}
		if b.loginStep(u, l) != "bind" {
			return errors.New("invalid_step")
		}
		if l.PendingCipher != "" {
			secret, err = b.unseal(l.PendingCipher)
			return err
		}
		secret, err = newMFASecret()
		if err != nil {
			return err
		}
		encrypted, err := b.seal(secret)
		if err != nil {
			return err
		}
		return tx.Model(&l).Update("pending_cipher", encrypted).Error
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	u, _, err := b.loginStateDB(b.DB, r)
	if err != nil {
		mutationError(w, r, err)
		return
	}
	uri := &url.URL{Scheme: "otpauth", Host: "totp", Path: "/Burrow:" + u.Username}
	q := url.Values{"secret": {secret}, "issuer": {"Burrow"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	uri.RawQuery = q.Encode()
	write(w, 200, map[string]string{"secret": secret, "uri": uri.String()})
}
func (b *Server) consumeOTP(tx *gorm.DB, u User, code string) error {
	secret, err := b.unseal(u.MFACipher)
	if err != nil {
		return err
	}
	step, ok := otpStep(secret, code, b.mfaTime(), u.MFALastStep)
	if !ok {
		return errors.New("invalid_mfa_code")
	}
	q := tx.Model(&User{}).Where("id = ? AND mfa_last_step = ? AND auth_version = ?", u.ID, u.MFALastStep, u.AuthVersion).Update("mfa_last_step", step)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return errors.New("invalid_mfa_code")
	}
	return nil
}
func (b *Server) loginVerify(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code string }
	if !decode(w, r, &in) {
		return
	}
	u, _, err := b.loginStateDB(b.DB, r)
	if err != nil {
		mutationError(w, r, err)
		return
	}
	if b.limited(r, "mfa") || b.limitedKey("mfa-account:"+u.ID, 10) {
		fail(w, r, 429, "rate_limited")
		return
	}
	var out map[string]any
	credential := ""
	rejected := false
	err = b.authenticationTx(func(tx *gorm.DB) error {
		u, l, err := b.loginStateDB(tx, r)
		if err != nil {
			return err
		}
		step := b.loginStep(u, l)
		switch step {
		case "verify":
			err = b.consumeOTP(tx, u, in.Code)
		case "bind":
			if l.PendingCipher == "" {
				return errors.New("invalid_step")
			}
			var secret string
			secret, err = b.unseal(l.PendingCipher)
			if err != nil {
				return err
			}
			accepted, ok := otpStep(secret, in.Code, b.mfaTime(), -1)
			if !ok {
				err = errors.New("invalid_mfa_code")
			} else {
				err = tx.Model(&User{}).Where("id = ? AND mfa_enabled = ?", u.ID, false).Updates(map[string]any{"mfa_enabled": true, "mfa_cipher": l.PendingCipher, "mfa_last_step": accepted}).Error
				u.MFAEnabled, u.MFACipher = true, l.PendingCipher
			}
		default:
			return errors.New("invalid_step")
		}
		if err != nil {
			if err.Error() != "invalid_mfa_code" {
				return err
			}
			rejected = true
			if err := tx.Model(&l).Updates(map[string]any{"attempts": l.Attempts + 1}).Error; err != nil {
				return err
			}
			return tx.Create(&Event{ID: random(18), ActorID: u.ID, ObjectID: u.ID, Kind: "mfa:verify", Success: false, RequestID: requestID(r), CreatedAt: time.Now()}).Error
		}
		l.MFAVerified, l.MFAAt, l.PendingCipher = true, time.Now(), ""
		if err := tx.Save(&l).Error; err != nil {
			return err
		}
		if step == "bind" {
			if err := tx.Create(&Event{ID: random(18), ActorID: u.ID, ObjectID: u.ID, Kind: "mfa:bind", Success: true, RequestID: requestID(r), CreatedAt: time.Now()}).Error; err != nil {
				return err
			}
		}
		out, credential, err = b.finishLogin(tx, u, l, r)
		return err
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	if rejected {
		fail(w, r, 400, "invalid_mfa_code")
		return
	}
	b.loginResponse(w, out, credential)
}
func (b *Server) finishLogin(tx *gorm.DB, u User, l LoginTransaction, r *http.Request) (map[string]any, string, error) {
	if b.loginStep(u, l) != "complete" {
		return b.loginResult(u, l), "", nil
	}
	credential := random(32)
	session := Session{ID: random(18), UserID: u.ID, CredentialHash: hash(credential), Method: "password", AuthTime: time.Now(), MFAAt: l.MFAAt, AuthVersion: u.AuthVersion, ExpiresAt: time.Now().Add(b.Config.SessionTTL)}
	if err := tx.Create(&session).Error; err != nil {
		return nil, "", err
	}
	if err := tx.Delete(&l).Error; err != nil {
		return nil, "", err
	}
	if err := tx.Create(&Event{ID: random(18), ActorID: u.ID, ObjectID: u.ID, Kind: "login", Success: true, RequestID: requestID(r), CreatedAt: time.Now()}).Error; err != nil {
		return nil, "", err
	}
	out := map[string]any{"step": "complete", "redirect": "/"}
	if l.RequestID != "" {
		out["redirect"] = "/oidc/login?requestId=" + url.QueryEscape(l.RequestID)
	}
	return out, credential, nil
}
func (b *Server) loginResponse(w http.ResponseWriter, out map[string]any, credential string) {
	if credential != "" {
		b.cookie(w, "burrow_login", "", -1)
		b.cookie(w, "burrow_session", credential, int(b.Config.SessionTTL.Seconds()))
	}
	write(w, 200, out)
}
func (b *Server) loginPassword(w http.ResponseWriter, r *http.Request) {
	current, login, err := b.loginStateDB(b.DB, r)
	if err != nil {
		mutationError(w, r, err)
		return
	}
	if b.loginStep(current, login) != "password" {
		fail(w, r, 400, "invalid_step")
		return
	}
	if b.limited(r, "login-password") || b.limitedKey("login-password-account:"+current.ID, 30) {
		fail(w, r, 429, "rate_limited")
		return
	}
	var in struct{ Password string }
	if !decode(w, r, &in) {
		return
	}
	h, err := passwordHash(in.Password)
	if err != nil {
		fail(w, r, 400, "password_policy")
		return
	}
	var out map[string]any
	credential, restricted := "", ""
	err = b.authenticationTx(func(tx *gorm.DB) error {
		u, l, err := b.loginStateDB(tx, r)
		if err != nil {
			return err
		}
		if b.loginStep(u, l) != "password" {
			return errors.New("invalid_step")
		}
		if err := invalidateAuthentication(tx, u.ID, l.ID, l.RequestID); err != nil {
			return err
		}
		u.PasswordHash, u.MustChangePassword, u.AuthVersion = h, false, u.AuthVersion+1
		if err := tx.Save(&u).Error; err != nil {
			return err
		}
		// Rotate the restricted credential without extending its original deadline.
		restricted = random(32)
		l.AuthVersion, l.CredentialHash = u.AuthVersion, hash(restricted)
		if err := tx.Save(&l).Error; err != nil {
			return err
		}
		if err := tx.Create(&Event{ID: random(18), ActorID: u.ID, ObjectID: u.ID, Kind: "password:change", Success: true, RequestID: requestID(r), CreatedAt: time.Now()}).Error; err != nil {
			return err
		}
		out, credential, err = b.finishLogin(tx, u, l, r)
		return err
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	if credential == "" {
		b.cookie(w, "burrow_login", restricted, int(restrictedLoginTTL.Seconds()))
	}
	b.loginResponse(w, out, credential)
}
func (b *Server) cancelLogin(r *http.Request) error {
	if c, err := r.Cookie("burrow_login"); err == nil {
		return b.DB.Where("credential_hash = ?", hash(c.Value)).Delete(&LoginTransaction{}).Error
	}
	return nil
}
func (b *Server) loginCancel(w http.ResponseWriter, r *http.Request) {
	if err := b.cancelLogin(r); err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	b.cookie(w, "burrow_login", "", -1)
	write(w, 200, map[string]bool{"ok": true})
}

// Keep only the transaction performing a forced password change, and its RP request.
func invalidateAuthentication(tx *gorm.DB, id, keepLogin, keepRequest string) error {
	if err := tx.Model(&Session{}).Where("user_id = ?", id).Update("revoked", true).Error; err != nil {
		return err
	}
	if err := tx.Model(&TokenRecord{}).Where("user_id = ?", id).Update("revoked", true).Error; err != nil {
		return err
	}
	pending := tx.Model(&LoginTransaction{}).Select("request_id").Where("user_id = ? AND id <> ? AND request_id <> ?", id, keepLogin, keepRequest)
	if err := tx.Where("id <> ? AND (user_id = ? OR id IN (?))", keepRequest, id, pending).Delete(&AuthTransaction{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ? AND id <> ?", id, keepLogin).Delete(&LoginTransaction{}).Error
}
func (s *Store) resetMFATx(tx *gorm.DB, target User, actor, reason, request string) error {
	if err := tx.Model(&User{}).Where("id = ?", target.ID).Updates(map[string]any{"mfa_enabled": false, "mfa_cipher": "", "mfa_last_step": -1, "auth_version": gorm.Expr("auth_version + 1")}).Error; err != nil {
		return err
	}
	if err := invalidateAuthentication(tx, target.ID, "", ""); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]string{"reason": reason})
	return tx.Create(&Event{ID: random(18), ActorID: actor, ObjectID: target.ID, Kind: "mfa:reset", Success: true, RequestID: request, Details: string(details), CreatedAt: time.Now()}).Error
}
func validResetReason(reason string) bool {
	return len(strings.TrimSpace(reason)) > 0 && len(reason) <= 500
}
func (b *Server) resetMFA(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := b.require(w, r, "")
	if !ok {
		return
	}
	var in struct{ Code, Reason string }
	if !decode(w, r, &in) {
		return
	}
	if !validResetReason(in.Reason) {
		fail(w, r, 400, "invalid_reason")
		return
	}
	if b.limited(r, "mfa-reset") || b.limitedKey("mfa-account:"+actor.ID, 10) {
		fail(w, r, 429, "rate_limited")
		return
	}
	err := b.authenticationTx(func(tx *gorm.DB) error {
		current, _, err := b.sessionDB(tx, r)
		if err != nil {
			return errors.New("unauthenticated")
		}
		_, admin, err := permissions(tx, current)
		if err != nil {
			return err
		}
		if !admin {
			return errors.New("forbidden")
		}
		if b.Config.MFAEnabled {
			if err := b.consumeOTP(tx, current, in.Code); err != nil {
				return err
			}
		}
		var target User
		if err := tx.First(&target, "id = ?", chi.URLParam(r, "id")).Error; err != nil {
			return err
		}
		return b.resetMFATx(tx, target, current.ID, strings.TrimSpace(in.Reason), requestID(r))
	})
	if err != nil {
		mutationError(w, r, err)
		return
	}
	write(w, 200, map[string]bool{"ok": true})
}

// ResetMFA is deliberately an operator-only CLI capability, never an anonymous API.
func (s *Store) ResetMFA(username, reason string) error {
	if strings.TrimSpace(username) == "" || !validResetReason(reason) {
		return errors.New("username and reason are required (reason at most 500 bytes)")
	}
	return s.authenticationTx(func(tx *gorm.DB) error {
		var target User
		if err := tx.Where("username = ?", username).First(&target).Error; err != nil {
			return err
		}
		return s.resetMFATx(tx, target, "operator:cli", strings.TrimSpace(reason), "")
	})
}
