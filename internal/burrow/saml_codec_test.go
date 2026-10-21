package burrow

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/xml"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/zitadel/saml/pkg/provider/signature"
	"github.com/zitadel/saml/pkg/provider/xml/samlp"
)

// Test-only keys are generated in memory and are never printed or persisted.
func testSAMLCertificate(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Burrow SAML test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

func testSAMLRedirect(t *testing.T, key *rsa.PrivateKey, relay *string, alternateEncoding bool) string {
	t.Helper()
	// Verification deliberately uses the exact wire bytes rather than a
	// regenerated query. The request is opaque to this signature-only test.
	request := url.QueryEscape(base64.StdEncoding.EncodeToString([]byte("test request")))
	if alternateEncoding {
		request = strings.ReplaceAll(request, "%3D", "%3d")
	}
	query := "SAMLRequest=" + request
	if relay != nil {
		query += "&RelayState=" + url.QueryEscape(*relay)
	}
	query += "&SigAlg=" + url.QueryEscape("http://www.w3.org/2001/04/xmldsig-more#rsa-sha256")
	digest := sha256.Sum256([]byte(query))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return query + "&Signature=" + url.QueryEscape(base64.StdEncoding.EncodeToString(sig))
}

func TestSAMLSDKRedirectSignature(t *testing.T) {
	key, cert := testSAMLCertificate(t)
	relay, empty := "transaction + /", ""
	for _, tc := range []struct {
		name      string
		relay     *string
		alternate bool
	}{
		{"relay", &relay, false}, {"absent-relay", nil, false},
		{"empty-relay", &empty, false}, {"original-encoding", &relay, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := testSAMLRedirect(t, key, tc.relay, tc.alternate)
			values, err := verifySAMLRedirect(query, cert)
			if err != nil || values.Get("SAMLRequest") == "" {
				t.Fatalf("valid Redirect signature rejected: %v", err)
			}
			_, present := values["RelayState"]
			if present != (tc.relay != nil) {
				t.Fatal("lost distinction between absent and empty RelayState")
			}
		})
	}
	query := testSAMLRedirect(t, key, &relay, false)
	for name, invalid := range map[string]string{
		"request-tampered":    strings.Replace(query, "SAMLRequest=", "SAMLRequest=eA", 1),
		"relay-tampered":      strings.Replace(query, "RelayState=", "RelayState=changed", 1),
		"missing-signature":   strings.Split(query, "&Signature=")[0],
		"sha1-algorithm":      strings.ReplaceAll(query, url.QueryEscape("http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"), url.QueryEscape("http://www.w3.org/2000/09/xmldsig#rsa-sha1")),
		"duplicate-request":   query + "&SAMLRequest=another",
		"duplicate-relay":     query + "&RelayState=another",
		"duplicate-algorithm": query + "&SigAlg=another",
		"duplicate-signature": query + "&Signature=another",
		"encoded-duplicate":   query + "&%53AMLRequest=another",
		"invalid-escape":      query + "&RelayState=%xx",
		"oversized-query":     query + strings.Repeat("x", 32*1024),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifySAMLRedirect(invalid, cert); err == nil {
				t.Fatal("invalid Redirect signature accepted")
			}
		})
	}
	t.Run("oversized-signed-relay", func(t *testing.T) {
		long := strings.Repeat("x", 81)
		if _, err := verifySAMLRedirect(testSAMLRedirect(t, key, &long, false), cert); err == nil {
			t.Fatal("oversized RelayState accepted")
		}
	})
	t.Run("missing-certificate", func(t *testing.T) {
		if _, err := verifySAMLRedirect(query, nil); err == nil {
			t.Fatal("missing certificate accepted")
		}
	})
	t.Run("wrong-key-type", func(t *testing.T) {
		other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		wrong := *cert
		wrong.PublicKey = &other.PublicKey
		if _, err := verifySAMLRedirect(query, &wrong); err == nil {
			t.Fatal("non-RSA key accepted")
		}
	})
	// Removing an explicitly signed empty RelayState changes the signature input.
	if _, err := verifySAMLRedirect(strings.Replace(testSAMLRedirect(t, key, &empty, false), "&RelayState=", "", 1), cert); err == nil {
		t.Fatal("empty RelayState removed without invalidating signature")
	}
}

func TestSAMLSDKDoubleSignature(t *testing.T) {
	key, cert := testSAMLCertificate(t)
	now := time.Now().UTC().Truncate(time.Second)
	input := samlResponseInput{
		Issuer: "https://sso.example.test/saml/metadata", Audience: "jenkins-test",
		ACSURL: "https://jenkins.example.test/securityRealm/finishLogin", RequestID: "_request",
		User:     User{ID: "stable-user", Username: "test-user", Name: "Test & User", Email: "test@example.test"},
		AuthTime: now.Add(-2 * time.Hour), IssuedAt: now, SessionExpiresAt: now.Add(time.Minute),
	}
	raw, err := buildSAMLResponse(input, key, cert)
	if err != nil {
		t.Fatal(err)
	}
	var response samlp.ResponseType
	if err := xml.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	assertion := response.Assertion
	if response.Status.StatusCode.Value != "urn:oasis:names:tc:SAML:2.0:status:Success" || response.Issuer == nil || response.Issuer.Format != "urn:oasis:names:tc:SAML:2.0:nameid-format:entity" {
		t.Fatal("incorrect SAML status or issuer format URI")
	}
	if response.InResponseTo != input.RequestID || response.Destination != input.ACSURL || assertion.Subject == nil || assertion.Subject.NameID == nil {
		t.Fatal("missing request or recipient binding")
	}
	if assertion.Subject.NameID.Text != input.User.ID || assertion.Subject.NameID.Format != "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent" {
		t.Fatal("unstable subject or incorrect NameID format")
	}
	if len(assertion.AuthnStatement) != 1 || assertion.AuthnStatement[0].AuthnInstant != input.AuthTime.Format(time.RFC3339Nano) {
		t.Fatal("shared-session authentication time was replaced by issue time")
	}
	statement := assertion.AuthnStatement[0]
	if statement.SessionNotOnOrAfter != input.SessionExpiresAt.Format(time.RFC3339Nano) || statement.SessionIndex == "" {
		t.Fatal("missing session expiry or opaque session index")
	}
	if assertion.Conditions == nil || assertion.Conditions.NotOnOrAfter != input.SessionExpiresAt.Format(time.RFC3339Nano) ||
		len(assertion.Conditions.AudienceRestriction) != 1 || len(assertion.Conditions.AudienceRestriction[0].Audience) != 1 || assertion.Conditions.AudienceRestriction[0].Audience[0] != input.Audience {
		t.Fatal("assertion exceeds session lifetime or has incorrect audience")
	}
	confirmation := assertion.Subject.SubjectConfirmation
	if len(confirmation) != 1 || confirmation[0].Method != "urn:oasis:names:tc:SAML:2.0:cm:bearer" {
		t.Fatal("incorrect subject confirmation method URI")
	}
	if len(confirmation) != 1 || confirmation[0].SubjectConfirmationData == nil || confirmation[0].SubjectConfirmationData.Recipient != input.ACSURL || confirmation[0].SubjectConfirmationData.InResponseTo != input.RequestID {
		t.Fatal("subject confirmation is not bound to the SP request")
	}
	if len(assertion.AttributeStatement) != 1 || len(assertion.AttributeStatement[0].Attribute) != 3 {
		t.Fatal("unexpected attributes disclosed")
	}
	attributes := map[string]string{}
	for _, attribute := range assertion.AttributeStatement[0].Attribute {
		if len(attribute.AttributeValue) != 1 {
			t.Fatal("attribute is not a single value")
		}
		attributes[attribute.Name] = attribute.AttributeValue[0]
	}
	if attributes["username"] != input.User.Username || attributes["displayName"] != input.User.Name || attributes["email"] != input.User.Email {
		t.Fatal("identity attributes changed during XML encoding")
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		t.Fatal(err)
	}
	root := doc.Root()
	child := root.SelectElement("Assertion")
	if child == nil || root.SelectAttrValue("ID", "") == child.SelectAttrValue("ID", "") {
		t.Fatal("missing assertion or non-unique XML IDs")
	}
	for _, element := range []*etree.Element{root, child} {
		children := element.ChildElements()
		if len(element.SelectElements("Signature")) != 1 || len(children) < 2 || children[0].Tag != "Issuer" || children[1].Tag != "Signature" {
			t.Fatal("signature count or SAML schema order incorrect")
		}
		sig := element.SelectElement("Signature")
		if sig == nil || sig.FindElement("./SignedInfo/Reference") == nil || sig.FindElement("./SignedInfo/Reference").SelectAttrValue("URI", "") != "#"+element.SelectAttrValue("ID", "") {
			t.Fatal("signature does not reference its own element")
		}
		if sig.FindElement("./SignedInfo/SignatureMethod").SelectAttrValue("Algorithm", "") != "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256" || sig.FindElement("./SignedInfo/Reference/DigestMethod").SelectAttrValue("Algorithm", "") != "http://www.w3.org/2001/04/xmlenc#sha256" {
			t.Fatal("signature or digest algorithm differs from the declared policy")
		}
		if err := signature.ValidatePost([]*x509.Certificate{cert}, element.Copy()); err != nil {
			t.Fatalf("%s XML signature rejected: %v", element.Tag, err)
		}
	}
	t.Run("response-tampered", func(t *testing.T) {
		changed := root.Copy()
		changed.CreateAttr("Destination", "https://other.example.test/acs")
		if err := signature.ValidatePost([]*x509.Certificate{cert}, changed); err == nil {
			t.Fatal("modified response signature accepted")
		}
	})
	t.Run("assertion-tampered", func(t *testing.T) {
		changed := child.Copy()
		changed.FindElement("./Subject/NameID").SetText("another-user")
		if err := signature.ValidatePost([]*x509.Certificate{cert}, changed); err == nil {
			t.Fatal("modified assertion signature accepted")
		}
	})
	t.Run("expired-session", func(t *testing.T) {
		input.SessionExpiresAt = now
		if _, err := buildSAMLResponse(input, key, cert); err == nil {
			t.Fatal("expired session emitted an assertion")
		}
	})
}

func TestSAMLXMLStructure(t *testing.T) {
	space := "urn:oasis:names:tc:SAML:2.0:protocol"
	valid := `<AuthnRequest xmlns="` + space + `" ID="_request"/>`
	if err := validateSAMLXML([]byte(valid), space, "AuthnRequest"); err != nil {
		t.Fatalf("valid request XML rejected: %v", err)
	}
	for name, raw := range map[string]string{
		"duplicate-id":    `<AuthnRequest xmlns="` + space + `" ID="_request"><Nested ID="_request"/></AuthnRequest>`,
		"doctype":         `<!DOCTYPE AuthnRequest [<!ENTITY test "value">]>` + valid,
		"second-root":     valid + valid,
		"wrong-root":      `<Response xmlns="` + space + `"/>`,
		"wrong-namespace": `<AuthnRequest xmlns="https://other.example.test"/>`,
		"depth":           `<AuthnRequest xmlns="` + space + `">` + strings.Repeat("<Nested>", 32) + strings.Repeat("</Nested>", 32) + `</AuthnRequest>`,
		"size":            `<AuthnRequest xmlns="` + space + `">` + strings.Repeat("x", 256*1024) + `</AuthnRequest>`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateSAMLXML([]byte(raw), space, "AuthnRequest"); err == nil {
				t.Fatal("invalid XML accepted")
			}
		})
	}
}
