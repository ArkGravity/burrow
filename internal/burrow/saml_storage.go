package burrow

import (
	"errors"
	"gorm.io/gorm"
	"time"
)

func samlRequestDeadlines(receivedAt, issueInstant time.Time) (time.Time, time.Time, error) {
	if receivedAt.IsZero() || issueInstant.IsZero() || issueInstant.Before(receivedAt.Add(-5*time.Minute)) || issueInstant.After(receivedAt.Add(time.Minute)) {
		return time.Time{}, time.Time{}, errors.New("invalid_saml_request_time")
	}
	end := issueInstant.Add(6 * time.Minute)
	expires := receivedAt.Add(5 * time.Minute)
	if end.Before(expires) {
		expires = end
	}
	replay := end
	if expires.After(replay) {
		replay = expires
	}
	return expires, replay, nil
}
func (b *Server) registerSAMLRequest(config SAMLApplication, externalID, browserHash, relay string, force bool, failure string, received, issued time.Time) (SAMLTransaction, error) {
	received, issued = received.UTC(), issued.UTC()
	bad := errors.New("invalid_transaction")
	expires, replay, err := samlRequestDeadlines(received, issued)
	if err != nil {
		return SAMLTransaction{}, err
	}
	if externalID == "" || len(externalID) > 256 || browserHash == "" || len(relay) > 80 || config.EntityID == "" || len(config.EntityID) > 1024 || !contains([]string{"", "NoPassive", "InvalidNameIDPolicy", "NoAuthnContext", "RequestDenied"}, failure) {
		return SAMLTransaction{}, bad
	}
	st := SAMLTransaction{ID: random(24), ApplicationID: config.ApplicationID, SPRequestID: externalID, BrowserHash: browserHash, RelayState: relay, ACSURL: config.ACSURL, ConfigVersion: config.ConfigVersion, ForceAuthn: force, FailureStatus: failure, ReceivedAt: received, ExpiresAt: expires}
	err = b.authenticationTx(func(tx *gorm.DB) error {
		var app Application
		if err := tx.Where("id = ? AND enabled = ? AND protocol = ?", config.ApplicationID, true, "saml").First(&app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return bad
			}
			return err
		}
		var current SAMLApplication
		if err := tx.First(&current, "application_id = ?", app.ID).Error; err != nil {
			return err
		}
		if current.ConfigVersion != config.ConfigVersion || current.EntityID != config.EntityID || current.ACSURL != config.ACSURL || current.SigningCertificate != config.SigningCertificate {
			return bad
		}
		if err := tx.Where("sp_entity_id = ? AND sp_request_id = ? AND replay_until <= ?", config.EntityID, externalID, received).Delete(&SAMLRequestReplay{}).Error; err != nil {
			return err
		}
		guard := SAMLRequestReplay{SPEntityID: config.EntityID, SPRequestID: externalID, ReplayUntil: replay}
		if err := tx.Create(&guard).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return bad
			}
			return err
		}
		if err := tx.Create(&st).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return bad
			}
			return err
		}
		return nil
	})
	return st, err
}
func consumeSAMLTransaction(tx *gorm.DB, request SAMLTransaction, config SAMLApplication, now time.Time, actor string, success bool, requestID string) error {
	now = now.UTC()
	q := tx.Model(&SAMLTransaction{}).Where("id = ? AND browser_hash = ? AND consumed = ? AND expires_at > ? AND config_version = ? AND application_id = ? AND config_version = (SELECT config_version FROM saml_applications WHERE application_id = ?)", request.ID, request.BrowserHash, false, now, config.ConfigVersion, config.ApplicationID, config.ApplicationID).Update("consumed", true)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return errors.New("invalid_transaction")
	}
	return tx.Create(&Event{ID: random(18), ActorID: actor, ObjectID: config.ApplicationID, Kind: "saml:login", Success: success, RequestID: requestID, CreatedAt: now}).Error
}
