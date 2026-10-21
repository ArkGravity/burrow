package burrow

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestSAMLKeys(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s := testStore(t, driver)
			if err := s.EnsureSAMLSigningKey(); err != nil {
				t.Fatal(err)
			}
			var original SAMLSigningKey
			if err := s.DB.Where("active = ?", true).First(&original).Error; err != nil {
				t.Fatal(err)
			}
			private, cert, err := s.samlSigningKey(s.DB, time.Now())
			if err != nil || private.N.BitLen() != 3072 || cert.NotAfter.Sub(cert.NotBefore) < 365*24*time.Hour {
				t.Fatal("invalid generated signing key", err)
			}
			if strings.Contains(original.PrivateCipher, "PRIVATE KEY") || strings.Contains(original.PrivateCipher, "CERTIFICATE") {
				t.Fatal("unencrypted private key")
			}
			if err := s.EnsureSAMLSigningKey(); err != nil {
				t.Fatal(err)
			}
			var count int64
			s.DB.Model(&SAMLSigningKey{}).Count(&count)
			if count != 1 {
				t.Fatal("initialization not idempotent")
			}
			if err := s.DB.Model(&original).Update("certificate_pem", "damaged certificate").Error; err != nil {
				t.Fatal(err)
			}
			if err := s.EnsureSAMLSigningKey(); err == nil {
				t.Fatal("damaged certificate silently replaced")
			}
			if err := s.DB.Model(&original).Update("certificate_pem", string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))).Error; err != nil {
				t.Fatal(err)
			}
			wrong := &Store{DB: s.DB, Config: s.Config}
			wrong.Config.MasterKey[0] ^= 1
			if _, _, err := wrong.samlSigningKey(wrong.DB, time.Now()); err == nil {
				t.Fatal("wrong master key accepted")
			}
			if err := wrong.EnsureSAMLSigningKey(); err == nil {
				t.Fatal("bad existing key replaced")
			}
			if _, _, err := s.samlSigningKey(s.DB, cert.NotAfter.Add(time.Second)); err == nil {
				t.Fatal("expired key accepted")
			}
			if err := s.RotateSAMLKeys(""); err == nil {
				t.Fatal("missing audit reason accepted")
			}
			if err := s.RotateKeys(); err != nil {
				t.Fatal(err)
			}
			var oidc SigningKey
			s.DB.Where("active = ?", true).First(&oidc)
			// Audit failure must roll back key replacement and keep the old key usable.
			callback := "test:saml_audit_failure"
			s.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "events" {
					tx.AddError(gorm.ErrInvalidData)
				}
			})
			if err := s.RotateSAMLKeys("test rollback"); err == nil {
				t.Fatal("audit failure ignored")
			}
			s.DB.Callback().Create().Remove(callback)
			var current SAMLSigningKey
			s.DB.Where("active = ?", true).First(&current)
			if current.ID != original.ID {
				t.Fatal("audit failure changed active key")
			}
			if err := s.RotateSAMLKeys("test rotation"); err != nil {
				t.Fatal(err)
			}
			s.DB.Model(&SAMLSigningKey{}).Count(&count)
			if count != 2 {
				t.Fatal("historical public certificate lost")
			}
			if err := s.DB.First(&original, "id = ?", original.ID).Error; err != nil || original.Active {
				t.Fatal("old key not retained/inactivated")
			}
			var after SigningKey
			s.DB.Where("active = ?", true).First(&after)
			if after.ID != oidc.ID {
				t.Fatal("SAML rotation changed OIDC key")
			}
			if _, _, err := s.samlSigningKey(s.DB, time.Now()); err != nil {
				t.Fatal(err)
			}
			other, _ := testSAMLCertificate(t)
			der, _ := x509.MarshalPKCS8PrivateKey(other)
			cipher, _ := s.seal(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
			s.DB.Model(&SAMLSigningKey{}).Where("active = ?", true).Update("private_cipher", cipher)
			if _, _, err := s.samlSigningKey(s.DB, time.Now()); err == nil {
				t.Fatal("mismatched key accepted")
			}
			if err := s.DB.Model(&SAMLSigningKey{}).Where("active = ?", true).Update("active", false).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.EnsureSAMLSigningKey(); err == nil {
				t.Fatal("inactive key history silently reinitialized")
			}
			s.DB.Model(&SAMLSigningKey{}).Count(&count)
			if count != 2 {
				t.Fatal("initialization changed historical keys")
			}
		})
	}
}
