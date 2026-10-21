package burrow

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func installTestSchemaV5(t *testing.T, s *Store) {
	t.Helper()
	if err := s.DB.Migrator().CreateTable(&SchemaVersion{}); err != nil {
		t.Fatal(err)
	}
	old := []string{initialMigration, pkceCompatibilityMigration, customRolesMigration, removeUpstreamMigration, mandatoryMFAMigration}
	for _, migration := range old {
		for statement := range strings.SplitSeq(migration, ";") {
			if strings.TrimSpace(statement) != "" {
				if err := s.DB.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := s.DB.Create(&SchemaVersion{ID: 1, Version: 5, Checksum: hash(strings.Join(old, ""))}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestSAMLMigration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := openTestStore(t, driver)
			installTestSchemaV5(t, s)
			if err := s.DB.Table("applications").Create(map[string]any{
				"id": "existing-app", "client_id": "existing-client", "client_type": "web", "enabled": true,
				"name": "Existing", "secret_hash": "keep-secret", "redirect_urls": `["https://client.test/callback"]`,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Create(&User{ID: "existing-user", Username: "existing-user", Enabled: true, PasswordHash: "keep-password", AuthVersion: 7}).Error; err != nil {
				t.Fatal(err)
			}
			session := Session{ID: "existing-session", UserID: "existing-user", CredentialHash: "keep-session", Method: "password", AuthVersion: 7, ExpiresAt: time.Now().Add(time.Hour)}
			if err := s.DB.Omit("UTCTimestamps").Create(&session).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Create(&AuthTransaction{ID: "existing-request", ClientID: "existing-app", ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Create(&SigningKey{ID: "existing-key", Active: true, PrivateCipher: "keep-key", PublicJSON: "keep-public"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.Migrate(); err != nil {
				t.Fatal(err)
			}
			var version SchemaVersion
			if err := s.DB.First(&version, 1).Error; err != nil || version.Version != 6 {
				t.Fatalf("expected schema v6, got %d: %v", version.Version, err)
			}
			if err := s.Migrate(); err != nil {
				t.Fatalf("migration not idempotent: %v", err)
			}
			var app Application
			if err := s.DB.First(&app, "id = ?", "existing-app").Error; err != nil || app.ClientID != "existing-client" || app.SecretHash != "keep-secret" {
				t.Fatal("existing application credentials changed")
			}
			var protocol string
			if err := s.DB.Table("applications").Select("protocol").Where("id = ?", app.ID).Scan(&protocol).Error; err != nil || protocol != "oidc" {
				t.Fatal("existing application was not marked OIDC")
			}
			var user User
			if err := s.DB.First(&user, "id = ?", "existing-user").Error; err != nil || user.PasswordHash != "keep-password" || user.AuthVersion != 7 {
				t.Fatal("existing user authentication changed")
			}
			if err := s.DB.First(&session, "id = ?", session.ID).Error; err != nil || session.Revoked || session.CredentialHash != "keep-session" {
				t.Fatal("existing shared session revoked")
			}
			var key SigningKey
			if err := s.DB.First(&key, "id = ?", "existing-key").Error; err != nil || key.PrivateCipher != "keep-key" || key.PublicJSON != "keep-public" || !key.Active {
				t.Fatal("existing OIDC key changed")
			}
			var count int64
			if err := s.DB.Model(&AuthTransaction{}).Where("id = ?", "existing-request").Count(&count).Error; err != nil || count != 1 {
				t.Fatal("existing OIDC request removed")
			}
			if err := s.Health(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSAMLMigrationReplayConstraints(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := testStore(t, driver)
			insert := func(db *gorm.DB) error {
				return db.Table("saml_request_replays").Create(map[string]any{
					"sp_entity_id": "https://jenkins.test", "sp_request_id": "_request", "replay_until": time.Now().Add(time.Minute),
				}).Error
			}
			if err := insert(s.DB); err != nil {
				t.Fatalf("missing replay table: %v", err)
			}
			if err := insert(s.DB); err == nil {
				t.Fatal("duplicate SP request accepted")
			}
			app := Application{ID: "saml-app", Protocol: "saml", ClientID: "internal-client", ClientType: "saml", Enabled: false}
			if err := s.DB.Create(&app).Error; err != nil {
				t.Fatal(err)
			}
			config := SAMLApplication{ApplicationID: app.ID, EntityID: "https://jenkins.test", ACSURL: "https://jenkins.test/acs", SigningCertificate: "public", MetadataXML: "metadata", ConfigVersion: 1}
			if err := s.DB.Create(&config).Error; err != nil {
				t.Fatal(err)
			}
			pending := SAMLTransaction{ID: "pending", ApplicationID: app.ID, SPRequestID: "_request", BrowserHash: "browser", ACSURL: config.ACSURL, ConfigVersion: 1, ReceivedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
			if err := s.DB.Create(&pending).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Delete(&app).Error; err != nil {
				t.Fatal(err)
			}
			for _, model := range []any{&SAMLApplication{}, &SAMLTransaction{}} {
				var count int64
				if err := s.DB.Model(model).Count(&count).Error; err != nil || count != 0 {
					t.Fatal("application delete did not cascade", err)
				}
			}
			if err := insert(s.DB); err == nil {
				t.Fatal("application deletion removed replay guard")
			}
			for _, id := range []string{"one", "two"} {
				err := s.DB.Table("saml_signing_keys").Create(map[string]any{
					"id": id, "private_cipher": "encrypted-test", "certificate_pem": "public-test", "active": true, "created_at": time.Now(),
				}).Error
				if (id == "one" && err != nil) || (id == "two" && err == nil) {
					t.Fatal("single active signing key constraint incorrect")
				}
			}
		})
	}
}
