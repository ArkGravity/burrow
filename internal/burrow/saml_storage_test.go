package burrow

import (
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSAMLRequestDeadlines(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name                 string
		offset, life, replay time.Duration
		bad                  bool
	}{
		{"current", 0, 5 * time.Minute, 6 * time.Minute, false}, {"future-boundary", time.Minute, 5 * time.Minute, 7 * time.Minute, false}, {"old-boundary", -5 * time.Minute, time.Minute, time.Minute, false}, {"too-future", time.Minute + time.Nanosecond, 0, 0, true}, {"too-old", -5*time.Minute - time.Nanosecond, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expires, replay, err := samlRequestDeadlines(now, now.Add(tc.offset))
			if (err != nil) != tc.bad {
				t.Fatal("incorrect freshness decision", err)
			}
			if tc.bad {
				if !expires.IsZero() || !replay.IsZero() {
					t.Fatal("invalid request got deadlines")
				}
				return
			}
			if expires.Sub(now) != tc.life || replay.Sub(now) != tc.replay {
				t.Fatal("incorrect deadlines")
			}
		})
	}
}

func TestSAMLRequestRegistration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b := &Server{Store: testStore(t, driver)}
			a := Application{ID: random(18), ClientID: random(18), Protocol: "saml", ClientType: "saml", Enabled: true}
			if err := b.DB.Create(&a).Error; err != nil {
				t.Fatal(err)
			}
			config := SAMLApplication{ApplicationID: a.ID, EntityID: "https://sp.test", ACSURL: "https://sp.test/acs", SigningCertificate: "public", MetadataXML: "metadata", ConfigVersion: 1}
			if err := b.DB.Create(&config).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			st, err := b.registerSAMLRequest(config, "_request", "browser", "opaque", false, "", now, now.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.registerSAMLRequest(config, "_request", "other-browser", "opaque", false, "", now, now); err == nil {
				t.Fatal("duplicate request accepted")
			}
			if _, _, _, err := b.samlTransactionDB(b.DB, st.ID, "other-browser", now); err == nil {
				t.Fatal("browser binding ignored")
			}
			if _, _, _, err := b.samlTransactionDB(b.DB, st.ID, "browser", st.ExpiresAt); err == nil {
				t.Fatal("expiry ignored")
			}
			// Even deleting the local request and application cannot delete the replay guard.
			if err := b.DB.Delete(&a).Error; err != nil {
				t.Fatal(err)
			}
			if err := b.DB.Create(&a).Error; err != nil {
				t.Fatal(err)
			}
			if err := b.DB.Create(&config).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := b.registerSAMLRequest(config, "_request", "browser", "", false, "", now.Add(5*time.Minute+time.Second), now.Add(time.Minute)); err == nil {
				t.Fatal("replay accepted after transaction expired / application recreation")
			}
			config.ConfigVersion = 2
			b.DB.Save(&config)
			old := config
			old.ConfigVersion = 1
			if _, err := b.registerSAMLRequest(old, "_new", "browser", "", false, "", now, now); err == nil {
				t.Fatal("stale trust configuration accepted")
			}
		})
	}
}

func TestSAMLReplayRetention(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, admin, user := testServer(t, driver)
			_, config := testSAMLHTTPApp(t, b, admin)
			now := time.Now().UTC()
			issued := now.Add(time.Minute)
			st, err := b.registerSAMLRequest(config, "_cleanup", "browser", "", false, "", now, issued)
			if err != nil {
				t.Fatal(err)
			}
			after := st.ExpiresAt.Add(time.Second)
			if err := b.DB.Where("expires_at < ?", after).Delete(&SAMLTransaction{}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := b.registerSAMLRequest(config, "_cleanup", "browser", "", false, "", after, issued); err == nil {
				t.Fatal("cleanup shortened replay protection")
			}
			for _, operation := range []string{"rotation", "reset"} {
				id := "_" + operation
				pending, err := b.registerSAMLRequest(config, id, "browser", "", false, "", now, now)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.DB.Model(&pending).Update("user_id", user.ID).Error; err != nil {
					t.Fatal(err)
				}
				if operation == "rotation" {
					err = b.RotateSAMLKeys("replay retention test")
				} else {
					err = b.ResetMFA(user.Username, "replay retention test")
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := b.registerSAMLRequest(config, id, "browser", "", false, "", now, now); err == nil {
					t.Fatal(operation + " shortened replay protection")
				}
			}
			other := Application{ID: random(18), ClientID: random(18), Protocol: "saml", ClientType: "saml", Enabled: true}
			if err := b.DB.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			second := config
			second.ApplicationID, second.EntityID = other.ID, "https://other-sp.test"
			if err := b.DB.Create(&second).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := b.registerSAMLRequest(second, "_cleanup", "browser", "", false, "", now, now); err != nil {
				t.Fatal("independent SP cannot use same request ID", err)
			}
		})
	}
}

func TestSAMLAtomicConsume(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b := &Server{Store: testStore(t, driver)}
			a := Application{ID: random(18), ClientID: random(18), Protocol: "saml", ClientType: "saml", Enabled: true}
			b.DB.Create(&a)
			config := SAMLApplication{ApplicationID: a.ID, EntityID: "https://sp.test", ACSURL: "https://sp.test/acs", SigningCertificate: "public", MetadataXML: "metadata", ConfigVersion: 1}
			b.DB.Create(&config)
			now := time.Now()
			st, err := b.registerSAMLRequest(config, "_request", "browser", "", false, "", now, now)
			if err != nil {
				t.Fatal(err)
			}
			name := "test:saml_consume_audit"
			b.DB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "events" {
					tx.AddError(gorm.ErrInvalidData)
				}
			})
			if err := b.authenticationTx(func(tx *gorm.DB) error { return consumeSAMLTransaction(tx, st, config, time.Now(), "", true, "") }); err == nil {
				t.Fatal("audit failure ignored")
			}
			b.DB.Callback().Create().Remove(name)
			var current SAMLTransaction
			b.DB.First(&current, "id = ?", st.ID)
			if current.Consumed {
				t.Fatal("audit failure consumed request")
			}
			var success atomic.Int32
			var wg sync.WaitGroup
			for range 2 {
				wg.Go(func() {
					if b.authenticationTx(func(tx *gorm.DB) error { return consumeSAMLTransaction(tx, st, config, time.Now(), "", true, "") }) == nil {
						success.Add(1)
					}
				})
			}
			wg.Wait()
			if success.Load() != 1 {
				t.Fatal("request not consumed exactly once")
			}
			var count int64
			b.DB.Model(&Event{}).Where("kind = ?", "saml:login").Count(&count)
			if count != 1 {
				t.Fatal("audit count differs from successful consumption")
			}
		})
	}
}
