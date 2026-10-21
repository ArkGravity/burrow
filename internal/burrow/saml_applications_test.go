package burrow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func testSAMLMetadata(t *testing.T, entity, acs string) string {
	t.Helper()
	_, cert := testSAMLCertificate(t)
	return fmt.Sprintf(`<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="%s"><SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol" AuthnRequestsSigned="true"><KeyDescriptor use="signing"><ds:KeyInfo xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:X509Data><ds:X509Certificate>%s</ds:X509Certificate></ds:X509Data></ds:KeyInfo></KeyDescriptor><AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="%s" index="0"/></SPSSODescriptor></EntityDescriptor>`, entity, base64.StdEncoding.EncodeToString(cert.Raw), acs)
}

func TestSAMLApplicationMetadata(t *testing.T) {
	b := &Server{Store: &Store{Config: Config{Env: "prod"}}}
	metadata := testSAMLMetadata(t, "https://jenkins.test/metadata", "https://jenkins.test/acs")
	for name, raw := range map[string]string{
		"doctype":              "<!DOCTYPE x [<!ENTITY x 'bad'>]>" + metadata,
		"multiple-sp":          strings.Replace(metadata, "</EntityDescriptor>", `<SPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"/></EntityDescriptor>`, 1),
		"multiple-acs":         strings.Replace(metadata, "</SPSSODescriptor>", `<AssertionConsumerService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://attacker.test/acs" index="1"/></SPSSODescriptor>`, 1),
		"multiple-cert":        strings.Replace(metadata, "</KeyDescriptor>", `</KeyDescriptor><KeyDescriptor use="signing"/>`, 1),
		"expired":              strings.Replace(metadata, "<SPSSODescriptor", `<SPSSODescriptor validUntil="2000-01-01T00:00:00Z"`, 1),
		"http":                 strings.Replace(metadata, "https://jenkins.test/acs", "http://jenkins.test/acs", 1),
		"wildcard":             strings.Replace(metadata, "https://jenkins.test/acs", "https://*.test/acs", 1),
		"wrong-protocol":       strings.Replace(metadata, "urn:oasis:names:tc:SAML:2.0:protocol", "urn:other", 1),
		"wrong-cert-namespace": strings.ReplaceAll(metadata, "http://www.w3.org/2000/09/xmldsig#", "urn:fake"),
		"size":                 strings.Repeat(" ", samlMaxXML) + metadata,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := b.parseSAMLMetadata(raw, time.Now()); err == nil {
				t.Fatal("unsafe metadata accepted")
			}
		})
	}
	if _, err := b.parseSAMLMetadata(metadata, time.Now()); err != nil {
		t.Fatal("valid metadata rejected", err)
	}
}

func TestSAMLProtocolIsolation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, u := testServer(t, driver)
			a := Application{ID: random(18), Protocol: "saml", ClientID: random(18), ClientType: "saml", Enabled: true, SecretHash: hash("secret"), RedirectURLs: []string{"https://client.test/cb"}}
			if err := b.DB.Create(&a).Error; err != nil {
				t.Fatal(err)
			}
			store := oidcStore{b}
			if _, err := store.GetClientByClientID(t.Context(), a.ClientID); err == nil {
				t.Fatal("SAML client loaded by OIDC")
			}
			if err := store.AuthorizeClientIDSecret(t.Context(), a.ClientID, "secret"); err == nil {
				t.Fatal("SAML client authenticated by OIDC")
			}
			var session Session
			b.DB.Where("user_id = ? AND revoked = ?", u.ID, false).First(&session)
			if _, _, err := store.authorized(b.DB, AuthTransaction{UserID: u.ID, SessionID: session.ID, ClientID: a.ID}); err == nil {
				t.Fatal("SAML token authorization succeeded")
			}
			r := httptest.NewRequest("POST", "/oidc/token", strings.NewReader("grant_type=authorization_code&code=fake"))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetBasicAuth(a.ClientID, "secret")
			w := httptest.NewRecorder()
			b.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatalf("unexpected token result %d", w.Code)
			}
		})
	}
}

func TestSAMLApplication(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, _ := testServer(t, driver)
			if err := b.EnsureSAMLSigningKey(); err != nil {
				t.Fatal(err)
			}
			metadata := testSAMLMetadata(t, "https://jenkins.test/securityRealm/metadata", "https://jenkins.test/securityRealm/finishLogin")
			body := map[string]any{"name": "Jenkins", "protocol": "saml", "saml": map[string]any{"metadataXml": metadata}}
			w := c.request("POST", "/api/v1/applications", body, true)
			if w.Code != 201 {
				t.Fatalf("create SAML: %d %s", w.Code, w.Body.String())
			}
			var app Application
			if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil {
				t.Fatal(err)
			}
			var stored Application
			if err := b.DB.First(&stored, "id = ?", app.ID).Error; err != nil || stored.Protocol != "saml" || stored.ClientType != "saml" || stored.SecretHash != "" {
				t.Fatal("incorrect SAML persistence", err)
			}
			if strings.Contains(w.Body.String(), stored.ClientID) || strings.Contains(w.Body.String(), "clientSecret") {
				t.Fatal("internal credentials exposed")
			}
			var config SAMLApplication
			if err := b.DB.First(&config, "application_id = ?", app.ID).Error; err != nil || config.EntityID != "https://jenkins.test/securityRealm/metadata" || config.ConfigVersion != 1 {
				t.Fatal("metadata not derived", err)
			}
			for _, bad := range []map[string]any{
				{"protocol": "oidc"}, {"clientId": "changed"}, {"clientType": "web"}, {"clientSecret": "unused-secret-123456"}, {"redirectUris": []string{}}, {"saml": map[string]any{"entityId": "attacker"}},
			} {
				if r := c.request("PUT", "/api/v1/applications/"+app.ID, bad, true); r.Code != 400 {
					t.Fatalf("mixed/immutable input accepted: %s", r.Body.String())
				}
			}
			if r := c.request("POST", "/api/v1/applications/"+app.ID+"/secret", map[string]any{}, true); r.Code != 400 {
				t.Fatal("SAML secret rotation accepted")
			}
			if r := c.request("POST", "/api/v1/applications", body, true); r.Code != 409 {
				t.Fatalf("duplicate entity accepted: %d %s", r.Code, r.Body.String())
			}
			if r := c.request("GET", "/oidc/authorize?client_id="+stored.ClientID+"&redirect_uri="+config.ACSURL, nil, false); r.Code < 400 {
				t.Fatal("SAML client entered OIDC")
			}
			if _, err := (oidcStore{b}).GetClientByClientID(t.Context(), stored.ClientID); err == nil {
				t.Fatal("SAML OIDC client lookup succeeded")
			}
			if r := c.request("PUT", "/api/v1/applications/"+app.ID, map[string]any{"name": "Renamed"}, true); r.Code != 200 {
				t.Fatal(r.Body.String())
			}
			// A failed management audit must roll back the trust material,
			// version and invalidation of the outstanding protocol request.
			now := time.Now()
			pending, err := b.registerSAMLRequest(config, "_metadata_update", "browser", "", false, "", now, now)
			if err != nil {
				t.Fatal(err)
			}
			changed := testSAMLMetadata(t, config.EntityID, "https://jenkins.test/replacement-acs")
			callback := "test:saml_metadata_audit"
			b.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "events" {
					tx.AddError(gorm.ErrInvalidData)
				}
			})
			update := map[string]any{"saml": map[string]any{"metadataXml": changed}}
			if r := c.request("PUT", "/api/v1/applications/"+app.ID, update, true); r.Code != 503 {
				t.Fatalf("audit failure status: %d", r.Code)
			}
			b.DB.Callback().Create().Remove(callback)
			var preserved SAMLApplication
			if err := b.DB.First(&preserved, "application_id = ?", app.ID).Error; err != nil || preserved.MetadataXML != config.MetadataXML || preserved.ConfigVersion != config.ConfigVersion {
				t.Fatal("audit failure changed SP trust", err)
			}
			if _, _, _, err := b.samlTransactionDB(b.DB, pending.ID, "browser", now); err != nil {
				t.Fatal("audit failure revoked pending request", err)
			}
			if r := c.request("PUT", "/api/v1/applications/"+app.ID, update, true); r.Code != 200 {
				t.Fatal(r.Body.String())
			}
			if _, _, _, err := b.samlTransactionDB(b.DB, pending.ID, "browser", now); err == nil {
				t.Fatal("updated metadata retained pending request")
			}
			if err := b.DB.Where("active = ?", true).Delete(&SAMLSigningKey{}).Error; err != nil {
				t.Fatal(err)
			}
			body["name"] = "Another"
			body["saml"] = map[string]any{"metadataXml": testSAMLMetadata(t, "https://second.test/metadata", "https://second.test/acs")}
			if r := c.request("POST", "/api/v1/applications", body, true); r.Code != 503 {
				t.Fatalf("enabled without key: %d %s", r.Code, r.Body.String())
			}
			body["enabled"] = false
			if r := c.request("POST", "/api/v1/applications", body, true); r.Code != 201 {
				t.Fatalf("disabled config rejected: %s", r.Body.String())
			}
		})
	}
}
