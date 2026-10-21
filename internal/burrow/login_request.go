package burrow

import (
	"errors"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"time"
)

type LoginRequestRef struct{ Type, ID string }

func normalizeLoginRequest(kind, id string) (LoginRequestRef, error) {
	if kind == "" {
		kind = "oidc"
	}
	if kind != "oidc" && kind != "saml" {
		return LoginRequestRef{}, errors.New("invalid_request_type")
	}
	if kind == "saml" && id == "" {
		return LoginRequestRef{}, errors.New("invalid_transaction")
	}
	return LoginRequestRef{Type: kind, ID: id}, nil
}
func loginRequestURL(ref LoginRequestRef) (string, error) {
	ref, err := normalizeLoginRequest(ref.Type, ref.ID)
	if err != nil {
		return "", err
	}
	if ref.ID == "" {
		return "/", nil
	}
	return "/" + ref.Type + "/login?requestId=" + url.QueryEscape(ref.ID), nil
}

func (b *Server) samlTransactionDB(tx *gorm.DB, id, browserHash string, now time.Time) (SAMLTransaction, Application, SAMLApplication, error) {
	now = now.UTC()
	var st SAMLTransaction
	var app Application
	var config SAMLApplication
	bad := errors.New("invalid_transaction")
	if id == "" || browserHash == "" {
		return st, app, config, bad
	}
	if err := tx.Where("id = ? AND browser_hash = ? AND expires_at > ? AND consumed = ?", id, browserHash, now, false).First(&st).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return st, app, config, err
		}
		return st, app, config, bad
	}
	if err := tx.Where("id = ? AND protocol = ? AND enabled = ?", st.ApplicationID, "saml", true).First(&app).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return st, app, config, err
		}
		return st, app, config, bad
	}
	if err := tx.Where("application_id = ? AND config_version = ? AND acs_url = ?", app.ID, st.ConfigVersion, st.ACSURL).First(&config).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return st, app, config, err
		}
		return st, app, config, bad
	}
	return st, app, config, nil
}

func (b *Server) authContext(w http.ResponseWriter, r *http.Request) {
	ref, err := normalizeLoginRequest(r.URL.Query().Get("requestType"), r.URL.Query().Get("requestId"))
	if err != nil {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	name := ""
	if ref.ID != "" {
		var app Application
		if ref.Type == "saml" {
			cookie, e := r.Cookie("burrow_browser")
			if e != nil {
				fail(w, r, 400, "invalid_transaction")
				return
			}
			_, app, _, err = b.samlTransactionDB(b.DB, ref.ID, hash(cookie.Value), time.Now())
		} else {
			a, e := b.browserAuth(r, ref.ID)
			err = e
			if err == nil {
				err = b.DB.First(&app, "id = ? AND protocol = ? AND enabled = ?", a.ClientID, "oidc", true).Error
			}
		}
		if err != nil {
			fail(w, r, 400, "invalid_transaction")
			return
		}
		name = app.Name
	}
	write(w, 200, map[string]string{"applicationName": name})
}
