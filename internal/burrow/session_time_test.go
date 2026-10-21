package burrow

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionTimeCompatibility(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := testStore(t, driver)
			s.Config.MFAEnabled = false
			b := &Server{Store: s}
			u := User{ID: random(18), Username: random(18), Enabled: true, PasswordHash: "existing-password-hash", AuthVersion: 1}
			if err := s.DB.Create(&u).Error; err != nil {
				t.Fatal(err)
			}
			for _, utc := range []bool{false, true} {
				for _, expired := range []bool{false, true} {
					clock := time.Now()
					if utc {
						clock = clock.UTC()
					}
					expiry := clock.Add(time.Hour)
					if expired {
						expiry = clock.Add(-time.Minute)
					}
					credential := random(32)
					session := Session{ID: random(18), UserID: u.ID, CredentialHash: hash(credential), Method: "password", AuthTime: clock.Add(-time.Hour), AuthVersion: u.AuthVersion, ExpiresAt: expiry, UTCTimestamps: utc}
					if err := s.DB.Create(&session).Error; err != nil {
						t.Fatal(err)
					}
					r := httptest.NewRequest("GET", "/", nil)
					r.AddCookie(&http.Cookie{Name: "burrow_session", Value: credential})
					_, _, err := b.session(r)
					if (err != nil) != expired {
						t.Fatalf("UTC=%t expired=%t: %v", utc, expired, err)
					}
				}
			}
		})
	}
}
