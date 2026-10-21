package burrow

import (
	"bytes"
	"compress/flate"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/zitadel/saml/pkg/provider/signature"
)

func testSAMLHTTPApp(t *testing.T, b *Server, c *browser) (*rsa.PrivateKey, SAMLApplication) {
	t.Helper()
	if err := b.EnsureSAMLSigningKey(); err != nil {
		t.Fatal(err)
	}
	key, cert := testSAMLCertificate(t)
	metadata := testSAMLMetadata(t, "https://jenkins.test/securityRealm/metadata", "https://jenkins.test/securityRealm/finishLogin")
	metadata = regexp.MustCompile(`<ds:X509Certificate>[^<]+</ds:X509Certificate>`).ReplaceAllString(metadata, "<ds:X509Certificate>"+base64.StdEncoding.EncodeToString(cert.Raw)+"</ds:X509Certificate>")
	w := c.request("POST", "/api/v1/applications", map[string]any{"name": "Jenkins", "protocol": "saml", "saml": map[string]string{"metadataXml": metadata}}, true)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var app Application
	if err := json.Unmarshal(w.Body.Bytes(), &app); err != nil || app.SAML == nil {
		t.Fatal("missing SAML summary", err)
	}
	app.SAML.ApplicationID = app.ID
	return key, *app.SAML
}

func testSAMLAuthnXML(b *Server, config SAMLApplication, id, options string) string {
	return fmt.Sprintf(`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="%s" Version="2.0" IssueInstant="%s" Destination="%s/saml/sso" AssertionConsumerServiceURL="%s" ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" %s><saml:Issuer>%s</saml:Issuer></samlp:AuthnRequest>`, id, time.Now().UTC().Format(time.RFC3339Nano), b.Config.Issuer, config.ACSURL, options, config.EntityID)
}

func testSAMLQuery(t *testing.T, key *rsa.PrivateKey, raw, relay string) string {
	t.Helper()
	var compressed bytes.Buffer
	w, err := flate.NewWriter(&compressed, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	query := "SAMLRequest=" + url.QueryEscape(base64.StdEncoding.EncodeToString(compressed.Bytes())) + "&RelayState=" + url.QueryEscape(relay) + "&SigAlg=" + url.QueryEscape(samlRSASHA256)
	digest := sha256.Sum256([]byte(query))
	signed, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return query + "&Signature=" + url.QueryEscape(base64.StdEncoding.EncodeToString(signed))
}

func testSAMLForm(t *testing.T, body string) *etree.Document {
	t.Helper()
	match := regexp.MustCompile(`name="SAMLResponse" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("missing SAML response form")
	}
	raw, err := base64.StdEncoding.DecodeString(html.UnescapeString(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSAMLHTTP(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, c, user := testServer(t, driver)
			key, config := testSAMLHTTPApp(t, b, c)
			w := c.request("GET", "/saml/metadata", nil, false)
			if w.Code != 200 || !strings.Contains(w.Body.String(), b.Config.Issuer+"/saml/metadata") || strings.Contains(w.Body.String(), "SingleLogoutService") {
				t.Fatalf("metadata: %d %s", w.Code, w.Body.String())
			}
			if w := c.request("GET", "/saml/certificate", nil, false); w.Code != 200 || !strings.Contains(w.Body.String(), "BEGIN CERTIFICATE") {
				t.Fatal("missing public signing certificate")
			}
			for _, path := range []string{"/saml/logout", "/saml/attribute", "/saml/unknown"} {
				if w := c.request("GET", path, nil, false); w.Code != 404 {
					t.Fatalf("unsupported endpoint %s: %d", path, w.Code)
				}
			}
			if w := c.request("POST", "/saml/sso", map[string]string{}, false); w.Code != 405 {
				t.Fatalf("POST SSO supported: %d", w.Code)
			}
			authTime := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
			b.DB.Model(&Session{}).Where("user_id = ? AND revoked = ?", user.ID, false).Update("auth_time", authTime)
			query := testSAMLQuery(t, key, testSAMLAuthnXML(b, config, "_valid", ""), "opaque-relay")
			w = c.request("GET", "/saml/sso?"+query, nil, false)
			if w.Code != 200 {
				t.Fatalf("SSO %d %s", w.Code, w.Body.String())
			}
			doc := testSAMLForm(t, w.Body.String())
			root := doc.Root()
			assertion := root.SelectElement("Assertion")
			if root.SelectAttrValue("InResponseTo", "") != "_valid" || assertion == nil || assertion.SelectElement("AuthnStatement").SelectAttrValue("AuthnInstant", "") != authTime.Format(time.RFC3339Nano) {
				t.Fatal("response request/auth time mismatch")
			}
			_, cert, err := b.samlSigningKey(b.DB, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err := signature.ValidatePost([]*x509.Certificate{cert}, root.Copy()); err != nil {
				t.Fatal("invalid response signature", err)
			}
			if err := signature.ValidatePost([]*x509.Certificate{cert}, assertion.Copy()); err != nil {
				t.Fatal("invalid assertion signature", err)
			}
			if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "form-action "+config.ACSURL) {
				t.Fatal("missing form response protection")
			}
			if w := c.request("GET", "/saml/sso?"+query, nil, false); w.Code != 400 {
				t.Fatal("external request replay accepted")
			}
			badXML := strings.Replace(testSAMLAuthnXML(b, config, "_bad-acs", ""), config.ACSURL, "https://attacker.test/acs", 1)
			if w := c.request("GET", "/saml/sso?"+testSAMLQuery(t, key, badXML, ""), nil, false); w.Code != 400 || strings.Contains(w.Body.String(), "SAMLResponse") {
				t.Fatal("unregistered ACS got response")
			}
			passive := testSAMLQuery(t, key, testSAMLAuthnXML(b, config, "_passive", `IsPassive="true"`), "")
			w = c.request("GET", "/saml/sso?"+passive, nil, false)
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			failed := testSAMLForm(t, w.Body.String()).Root()
			if failed.SelectElement("Assertion") != nil || !strings.Contains(w.Body.String(), "SAMLResponse") || failed.SelectElement("Status").SelectElement("StatusCode").SelectElement("StatusCode").SelectAttrValue("Value", "") != "urn:oasis:names:tc:SAML:2.0:status:NoPassive" {
				t.Fatal("passive request not explicitly refused")
			}
			if err := signature.ValidatePost([]*x509.Certificate{cert}, failed.Copy()); err != nil {
				t.Fatal("unsigned failure response", err)
			}
			fresh := newBrowser(b)
			w = fresh.request("GET", "/saml/sso?"+testSAMLQuery(t, key, testSAMLAuthnXML(b, config, "_login", ""), ""), nil, false)
			if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "requestType=saml") {
				t.Fatalf("missing typed login redirect %d %s", w.Code, w.Header().Get("Location"))
			}
			if w := c.request("GET", "/saml/login?requestId=forged", nil, false); w.Code != 400 {
				t.Fatal("forged continuation accepted")
			}
		})
	}
}
