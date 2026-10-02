package burrow

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// authorizationTx serializes administrative decisions and their writes. Do not
// read the request body or perform network calls while holding this transaction.
func (b *Server) authorizationTx(r *http.Request, permission string, f func(*gorm.DB, User, Session) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.DB.Transaction(func(tx *gorm.DB) error {
		if b.Config.DBDriver == "postgres" {
			if e := tx.Exec("SELECT pg_advisory_xact_lock(734285622)").Error; e != nil {
				return e
			}
		}
		u, s, e := b.sessionDB(tx, r)
		if e != nil {
			return errors.New("unauthenticated")
		}
		if permission != "" {
			p, _, e := permissions(tx, u)
			if e != nil {
				return e
			}
			if u.MustChangePassword || !contains(p, permission) {
				return errors.New("forbidden")
			}
		}
		if e := f(tx, u, s); e != nil {
			return e
		}
		return tx.Create(&Event{ID: random(18), ActorID: u.ID, ObjectID: chi.URLParam(r, "id"), Kind: r.Method + " " + r.URL.Path, RequestID: requestID(r), Success: true, CreatedAt: time.Now()}).Error
	})
}
func (b *Server) sessionDB(db *gorm.DB, r *http.Request) (User, Session, error) {
	var u User
	var s Session
	c, e := r.Cookie("burrow_session")
	if e != nil {
		return u, s, e
	}
	if e = db.Where("credential_hash = ? AND revoked = ? AND expires_at > ?", hash(c.Value), false, time.Now()).First(&s).Error; e != nil {
		return u, s, e
	}
	if e = db.Where("id = ? AND enabled = ?", s.UserID, true).First(&u).Error; e != nil {
		return u, s, e
	}
	if s.Method != "password" || u.PasswordHash == "" {
		return u, s, errors.New("invalid_authentication_method")
	}
	return u, s, nil
}
func mutationError(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "unavailable"
	switch {
	case e.Error() == "unauthenticated":
		status, code = 401, "unauthenticated"
	case e.Error() == "forbidden":
		status, code = 403, "forbidden"
	case strings.HasPrefix(e.Error(), "invalid_"):
		status, code = 400, e.Error()
	case errors.Is(e, gorm.ErrRecordNotFound):
		status, code = 404, "not_found"
	case errors.Is(e, gorm.ErrDuplicatedKey):
		status, code = 409, "conflict"
	}
	fail(w, r, status, code)
}
