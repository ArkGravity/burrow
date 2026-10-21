package burrow

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (s *Store) newSAMLSigningKey() (SAMLSigningKey, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return SAMLSigningKey{}, err
	}
	now := time.Now().UTC()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return SAMLSigningKey{}, err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Burrow SAML signing"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, SignatureAlgorithm: x509.SHA256WithRSA}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return SAMLSigningKey{}, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return SAMLSigningKey{}, err
	}
	encrypted, err := s.seal(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})))
	if err != nil {
		return SAMLSigningKey{}, err
	}
	return SAMLSigningKey{ID: random(18), PrivateCipher: encrypted, CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})), Active: true, CreatedAt: now}, nil
}

// Initialization is explicit (migrate), never a side effect of serving requests.
func (s *Store) EnsureSAMLSigningKey() error {
	var count int64
	if err := s.DB.Model(&SAMLSigningKey{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		_, _, err := s.samlSigningKey(s.DB, time.Now())
		return err
	}
	candidate, err := s.newSAMLSigningKey()
	if err != nil {
		return err
	}
	return s.authenticationTx(func(tx *gorm.DB) error {
		if err := tx.Model(&SAMLSigningKey{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			_, _, err := s.samlSigningKey(tx, time.Now())
			return err
		}
		return tx.Create(&candidate).Error
	})
}

func (s *Store) RotateSAMLKeys(reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 1024 {
		return errors.New("invalid_reason")
	}
	candidate, err := s.newSAMLSigningKey()
	if err != nil {
		return err
	}
	return s.authenticationTx(func(tx *gorm.DB) error {
		if err := tx.Model(&SAMLSigningKey{}).Where("active = ?", true).Update("active", false).Error; err != nil {
			return err
		}
		if err := tx.Create(&candidate).Error; err != nil {
			return err
		}
		if err := tx.Where("consumed = ?", false).Delete(&SAMLTransaction{}).Error; err != nil {
			return err
		}
		return tx.Create(&Event{ID: random(18), ObjectID: candidate.ID, Kind: "saml:keys-rotate", Details: reason, Success: true, CreatedAt: time.Now()}).Error
	})
}

func (s *Store) samlSigningKey(db *gorm.DB, now time.Time) (*rsa.PrivateKey, *x509.Certificate, error) {
	invalid := errors.New("saml_signing_key_unavailable")
	var rows []SAMLSigningKey
	if err := db.Where("active = ?", true).Limit(2).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	if len(rows) != 1 {
		return nil, nil, invalid
	}
	row := rows[0]
	block, rest := pem.Decode([]byte(row.CertificatePEM))
	if block == nil || block.Type != "CERTIFICATE" || strings.TrimSpace(string(rest)) != "" {
		return nil, nil, invalid
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return nil, nil, invalid
	}
	plain, err := s.unseal(row.PrivateCipher)
	if err != nil {
		return nil, nil, invalid
	}
	block, rest = pem.Decode([]byte(plain))
	if block == nil || block.Type != "PRIVATE KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, nil, invalid
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, invalid
	}
	key, ok := parsed.(*rsa.PrivateKey)
	pub, certOK := cert.PublicKey.(*rsa.PublicKey)
	if !ok || !certOK || key.N.BitLen() < 3072 || key.Validate() != nil || !key.PublicKey.Equal(pub) {
		return nil, nil, invalid
	}
	return key, cert, nil
}
