package burrow

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/zitadel/saml/pkg/provider/signature"
	samlxml "github.com/zitadel/saml/pkg/provider/xml"
	"github.com/zitadel/saml/pkg/provider/xml/saml"
	"github.com/zitadel/saml/pkg/provider/xml/samlp"
)

const (
	samlProtocolNS       = "urn:oasis:names:tc:SAML:2.0:protocol"
	samlAssertionNS      = "urn:oasis:names:tc:SAML:2.0:assertion"
	samlRSASHA256        = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
	samlPersistentNameID = "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent"
	samlPasswordContext  = "urn:oasis:names:tc:SAML:2.0:ac:classes:PasswordProtectedTransport"
	samlMaxXML           = 256 * 1024
	samlMaxQuery         = 32 * 1024
)

type samlResponseInput struct {
	Issuer, Audience, ACSURL, RequestID  string
	User                                 User
	AuthTime, IssuedAt, SessionExpiresAt time.Time
}

func verifySAMLRedirect(rawQuery string, cert *x509.Certificate) (url.Values, error) {
	invalid := errors.New("invalid_saml_signature")
	if len(rawQuery) > samlMaxQuery || cert == nil {
		return nil, invalid
	}
	publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || publicKey.N == nil || publicKey.N.BitLen() < 2048 {
		return nil, invalid
	}
	values := url.Values{}
	wire := map[string]string{}
	for part := range strings.SplitSeq(rawQuery, "&") {
		name, encoded, found := strings.Cut(part, "=")
		if !found || (name != "SAMLRequest" && name != "RelayState" && name != "SigAlg" && name != "Signature") {
			return nil, invalid
		}
		if _, duplicate := wire[name]; duplicate {
			return nil, invalid
		}
		decoded, err := url.QueryUnescape(encoded)
		if err != nil {
			return nil, invalid
		}
		wire[name] = encoded
		values[name] = []string{decoded}
	}
	if values.Get("SAMLRequest") == "" || values.Get("SigAlg") != samlRSASHA256 || values.Get("Signature") == "" || len(values.Get("RelayState")) > 80 {
		return nil, invalid
	}
	// The Redirect signature covers the original encoded values in protocol
	// order. Re-encoding url.Values would change valid signed wire bytes.
	signed := "SAMLRequest=" + wire["SAMLRequest"]
	if relay, present := wire["RelayState"]; present {
		signed += "&RelayState=" + relay
	}
	signed += "&SigAlg=" + wire["SigAlg"]
	decodedSignature, err := base64.StdEncoding.Strict().DecodeString(values.Get("Signature"))
	if err != nil {
		return nil, invalid
	}
	if err := signature.ValidateRedirect(samlRSASHA256, []byte(signed), decodedSignature, publicKey); err != nil {
		return nil, invalid
	}
	return values, nil
}

func buildSAMLResponse(input samlResponseInput, key *rsa.PrivateKey, cert *x509.Certificate) ([]byte, error) {
	if key == nil || cert == nil || input.Issuer == "" || input.Audience == "" || input.ACSURL == "" || input.RequestID == "" || input.User.ID == "" ||
		input.AuthTime.IsZero() || input.IssuedAt.IsZero() || input.AuthTime.After(input.IssuedAt) || !input.SessionExpiresAt.After(input.IssuedAt) ||
		input.IssuedAt.Before(cert.NotBefore) || !input.IssuedAt.Before(cert.NotAfter) {
		return nil, errors.New("invalid_saml_response")
	}
	expires := input.IssuedAt.Add(3 * time.Minute)
	if input.SessionExpiresAt.Before(expires) {
		expires = input.SessionExpiresAt
	}
	format := func(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
	issuer := saml.NameIDType{Format: "urn:oasis:names:tc:SAML:2.0:nameid-format:entity", Text: input.Issuer}
	response := samlp.ResponseType{
		Id: "_" + random(24), InResponseTo: input.RequestID, Version: "2.0", IssueInstant: format(input.IssuedAt),
		Destination: input.ACSURL, Issuer: &issuer,
		Status: samlp.StatusType{StatusCode: samlp.StatusCodeType{Value: "urn:oasis:names:tc:SAML:2.0:status:Success"}},
		Assertion: saml.AssertionType{
			Id: "_" + random(24), Version: "2.0", IssueInstant: format(input.IssuedAt), Issuer: issuer,
			Subject: &saml.SubjectType{
				NameID: &saml.NameIDType{Format: samlPersistentNameID, Text: input.User.ID},
				SubjectConfirmation: []saml.SubjectConfirmationType{{
					Method: "urn:oasis:names:tc:SAML:2.0:cm:bearer",
					SubjectConfirmationData: &saml.SubjectConfirmationDataType{
						Recipient: input.ACSURL, InResponseTo: input.RequestID, NotOnOrAfter: format(expires),
					},
				}},
			},
			Conditions: &saml.ConditionsType{
				NotBefore: format(input.IssuedAt), NotOnOrAfter: format(expires),
				AudienceRestriction: []saml.AudienceRestrictionType{{Audience: []string{input.Audience}}},
			},
			AuthnStatement: []saml.AuthnStatementType{{
				AuthnInstant: format(input.AuthTime), SessionIndex: "_" + random(24),
				SessionNotOnOrAfter: format(input.SessionExpiresAt),
				AuthnContext:        saml.AuthnContextType{AuthnContextClassRef: samlPasswordContext},
			}},
			AttributeStatement: []saml.AttributeStatementType{{Attribute: []*saml.AttributeType{
				{Name: "username", AttributeValue: []string{input.User.Username}},
				{Name: "displayName", AttributeValue: []string{input.User.Name}},
				{Name: "email", AttributeValue: []string{input.User.Email}},
			}}},
		},
	}
	raw, err := samlxml.Marshal(response)
	if err != nil {
		return nil, err
	}
	return signSAMLDocument(raw, key, cert, true)
}

func buildSAMLFailure(input samlResponseInput, status string, key *rsa.PrivateKey, cert *x509.Certificate) ([]byte, error) {
	if !contains([]string{"NoPassive", "InvalidNameIDPolicy", "NoAuthnContext", "RequestDenied"}, status) || key == nil || cert == nil || input.Issuer == "" || input.RequestID == "" || input.ACSURL == "" {
		return nil, errors.New("invalid_saml_response")
	}
	response := struct {
		XMLName      xml.Name         `xml:"urn:oasis:names:tc:SAML:2.0:protocol Response"`
		ID           string           `xml:"ID,attr"`
		Version      string           `xml:"Version,attr"`
		IssueInstant string           `xml:"IssueInstant,attr"`
		Destination  string           `xml:"Destination,attr"`
		InResponseTo string           `xml:"InResponseTo,attr"`
		Issuer       saml.NameIDType  `xml:"urn:oasis:names:tc:SAML:2.0:assertion Issuer"`
		Status       samlp.StatusType `xml:"Status"`
	}{ID: "_" + random(24), Version: "2.0", IssueInstant: input.IssuedAt.UTC().Format(time.RFC3339Nano), Destination: input.ACSURL, InResponseTo: input.RequestID, Issuer: saml.NameIDType{Format: "urn:oasis:names:tc:SAML:2.0:nameid-format:entity", Text: input.Issuer}, Status: samlp.StatusType{StatusCode: samlp.StatusCodeType{Value: "urn:oasis:names:tc:SAML:2.0:status:Responder", StatusCode: &samlp.StatusCodeType{Value: "urn:oasis:names:tc:SAML:2.0:status:" + status}}}}
	raw, err := xml.Marshal(response)
	if err != nil {
		return nil, err
	}
	return signSAMLDocument(raw, key, cert, false)
}

func signSAMLDocument(raw []byte, key *rsa.PrivateKey, cert *x509.Certificate, withAssertion bool) ([]byte, error) {
	// Sign the final XML nodes through the SDK's public signing context. This
	// preserves XML escaping and namespace context across the nested signatures.
	tlsKey, err := signature.ParseTlsKeyPair(cert.Raw, key)
	if err != nil {
		return nil, err
	}
	signer, err := signature.GetSigningContext(tlsKey, samlRSASHA256)
	if err != nil {
		return nil, err
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		return nil, err
	}
	root := doc.Root()
	if withAssertion {
		assertion := root.SelectElement("Assertion")
		assertionSignature, err := signer.ConstructSignature(assertion, true)
		if err != nil {
			return nil, err
		}
		assertion.InsertChildAt(assertion.SelectElement("Issuer").Index()+1, assertionSignature)
	}
	responseSignature, err := signer.ConstructSignature(root, true)
	if err != nil {
		return nil, err
	}
	root.InsertChildAt(root.SelectElement("Issuer").Index()+1, responseSignature)
	return doc.WriteToBytes()
}

func validateSAMLXML(raw []byte, rootSpace, rootName string) error {
	invalid := errors.New("invalid_saml_xml")
	if len(raw) == 0 || len(raw) > samlMaxXML {
		return invalid
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	depth, roots := 0, 0
	ids := map[string]bool{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if depth != 0 || roots != 1 {
				return invalid
			}
			return nil
		}
		if err != nil {
			return invalid
		}
		switch v := token.(type) {
		case xml.Directive:
			return invalid
		case xml.ProcInst:
			if v.Target != "xml" || roots != 0 {
				return invalid
			}
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || v.Name.Space != rootSpace || v.Name.Local != rootName {
					return invalid
				}
			}
			depth++
			if depth > 32 {
				return invalid
			}
			attributes := map[xml.Name]bool{}
			for _, attr := range v.Attr {
				if attributes[attr.Name] {
					return invalid
				}
				attributes[attr.Name] = true
				if attr.Name.Local == "ID" {
					if attr.Value == "" || ids[attr.Value] {
						return invalid
					}
					ids[attr.Value] = true
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(v)) != "" {
				return invalid
			}
		}
	}
}
