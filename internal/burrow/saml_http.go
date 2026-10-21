package burrow

import (
	"bytes"
	"compress/flate"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/zitadel/saml/pkg/provider/xml/md"
	"github.com/zitadel/saml/pkg/provider/xml/samlp"
	"github.com/zitadel/saml/pkg/provider/xml/xml_dsig"
	"gorm.io/gorm"
)

func (b *Server) samlEntityID() string { return b.Config.Issuer + "/saml/metadata" }

func (b *Server) samlMetadata(w http.ResponseWriter, r *http.Request) {
	_, cert, err := b.samlSigningKey(b.DB, time.Now())
	if err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	metadata := md.EntityDescriptorType{EntityID: md.EntityIDType(b.samlEntityID()), IDPSSODescriptor: &md.IDPSSODescriptorType{
		WantAuthnRequestsSigned: "true", ProtocolSupportEnumeration: md.AnyURIListType(samlProtocolNS),
		KeyDescriptor: []md.KeyDescriptorType{{Use: md.KeyTypesSigning, KeyInfo: xml_dsig.KeyInfoType{X509Data: []xml_dsig.X509DataType{{X509Certificate: base64.StdEncoding.EncodeToString(cert.Raw)}}}}},
		NameIDFormat:  []string{samlPersistentNameID}, SingleSignOnService: []md.EndpointType{{Binding: samlRedirectBinding, Location: b.Config.Issuer + "/saml/sso"}},
	}}
	raw, err := xml.Marshal(metadata)
	if err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml; charset=utf-8")
	w.Write(raw)
}

func (b *Server) samlCertificate(w http.ResponseWriter, r *http.Request) {
	_, cert, err := b.samlSigningKey(b.DB, time.Now())
	if err != nil {
		fail(w, r, 503, "unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="burrow-saml-signing.crt"`)
	w.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
}

func decodeSAMLAuthnRequest(encoded string) (samlp.AuthnRequestType, error) {
	var request samlp.AuthnRequestType
	bad := errors.New("invalid_saml_request")
	compressed, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(compressed) > samlMaxQuery {
		return request, bad
	}
	source := bytes.NewReader(compressed)
	reader := flate.NewReader(source)
	defer reader.Close()
	raw, err := io.ReadAll(io.LimitReader(reader, samlMaxXML+1))
	if err != nil || source.Len() != 0 || validateSAMLXML(raw, samlProtocolNS, "AuthnRequest") != nil {
		return request, bad
	}
	doc := etree.NewDocument()
	if doc.ReadFromBytes(raw) != nil {
		return request, bad
	}
	root := doc.Root()
	if len(samlChildren(root, samlAssertionNS, "Issuer")) != 1 {
		return request, bad
	}
	seen := map[string]bool{}
	for _, child := range root.ChildElements() {
		if seen[child.Tag] {
			return request, bad
		}
		seen[child.Tag] = true
		if child.Tag == "Issuer" {
			if child.NamespaceURI() != samlAssertionNS {
				return request, bad
			}
			continue
		}
		if child.NamespaceURI() != samlProtocolNS || !contains([]string{"NameIDPolicy", "RequestedAuthnContext", "Subject", "Conditions", "Scoping", "Extensions"}, child.Tag) {
			return request, bad
		}
	}
	if xml.Unmarshal(raw, &request) != nil || request.Issuer == nil || request.Issuer.Text == "" || len(request.Issuer.Text) > 1024 {
		return request, bad
	}
	return request, nil
}

func validateSAMLRequest(request samlp.AuthnRequestType, config SAMLApplication, destination string, now time.Time) (bool, string, time.Time, error) {
	bad := errors.New("invalid_saml_request")
	issued, err := time.Parse(time.RFC3339Nano, request.IssueInstant)
	if err != nil {
		return false, "", issued, bad
	}
	if _, _, err := samlRequestDeadlines(now, issued); err != nil {
		return false, "", issued, bad
	}
	if request.Version != "2.0" || request.Destination != destination || request.Id == "" || len(request.Id) > 256 || request.Issuer.Text != config.EntityID || (request.ProtocolBinding != "" && request.ProtocolBinding != samlPOSTBinding) {
		return false, "", issued, bad
	}
	for i, c := range request.Id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (i > 0 && ((c >= '0' && c <= '9') || c == '-' || c == '.'))) {
			return false, "", issued, bad
		}
	}
	if request.AssertionConsumerServiceURL != "" && request.AssertionConsumerServiceIndex != "" {
		return false, "", issued, bad
	}
	if request.AssertionConsumerServiceURL != "" && request.AssertionConsumerServiceURL != config.ACSURL {
		return false, "", issued, bad
	}
	if request.AssertionConsumerServiceIndex != "" {
		doc := etree.NewDocument()
		if doc.ReadFromString(config.MetadataXML) != nil {
			return false, "", issued, bad
		}
		sp := samlChildren(doc.Root(), samlMetadataNS, "SPSSODescriptor")
		if len(sp) != 1 {
			return false, "", issued, bad
		}
		want, err := strconv.ParseUint(request.AssertionConsumerServiceIndex, 10, 16)
		if err != nil {
			return false, "", issued, bad
		}
		matched := false
		for _, acs := range samlChildren(sp[0], samlMetadataNS, "AssertionConsumerService") {
			index, e := strconv.ParseUint(acs.SelectAttrValue("index", ""), 10, 16)
			if e == nil && index == want && acs.SelectAttrValue("Binding", "") == samlPOSTBinding && acs.SelectAttrValue("Location", "") == config.ACSURL {
				matched = true
			}
		}
		if !matched {
			return false, "", issued, bad
		}
	}
	parseBool := func(value string) (bool, error) {
		if value == "" || value == "false" || value == "0" {
			return false, nil
		}
		if value == "true" || value == "1" {
			return true, nil
		}
		return false, bad
	}
	force, err := parseBool(request.ForceAuthn)
	if err != nil {
		return false, "", issued, bad
	}
	passive, err := parseBool(request.IsPassive)
	if err != nil {
		return false, "", issued, bad
	}
	if passive {
		return force, "NoPassive", issued, nil
	}
	if policy := request.NameIDPolicy; policy != nil && ((policy.Format != "" && policy.Format != samlPersistentNameID) || (policy.SPNameQualifier != "" && policy.SPNameQualifier != config.EntityID)) {
		return force, "InvalidNameIDPolicy", issued, nil
	}
	if context := request.RequestedAuthnContext; context != nil && ((context.Comparison != "" && context.Comparison != "exact") || len(context.AuthnContextDeclRef) > 0 || len(context.AuthnContextClassRef) != 1 || context.AuthnContextClassRef[0] != samlPasswordContext) {
		return force, "NoAuthnContext", issued, nil
	}
	if request.Subject != nil || request.Conditions != nil || request.Scoping != nil || request.Extensions != nil || request.AttributeConsumingServiceIndex != "" {
		return force, "RequestDenied", issued, nil
	}
	return force, "", issued, nil
}

func (b *Server) samlSSO(w http.ResponseWriter, r *http.Request) {
	if b.limited(r, "saml") {
		fail(w, r, 429, "rate_limited")
		return
	}
	if len(r.URL.RequestURI()) > samlMaxQuery {
		fail(w, r, 400, "invalid_saml_request")
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["SAMLRequest"]) != 1 {
		fail(w, r, 400, "invalid_saml_request")
		return
	}
	request, err := decodeSAMLAuthnRequest(query.Get("SAMLRequest"))
	if err != nil {
		fail(w, r, 400, "invalid_saml_request")
		return
	}
	var config SAMLApplication
	if err := b.DB.Joins("JOIN applications ON applications.id = saml_applications.application_id").Where("saml_applications.entity_id = ? AND applications.protocol = ? AND applications.enabled = ?", request.Issuer.Text, "saml", true).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(w, r, 400, "invalid_saml_request")
		} else {
			fail(w, r, 503, "unavailable")
		}
		return
	}
	block, rest := pem.Decode([]byte(config.SigningCertificate))
	if block == nil || strings.TrimSpace(string(rest)) != "" {
		fail(w, r, 503, "unavailable")
		return
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	now := time.Now()
	if err != nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		fail(w, r, 400, "invalid_saml_request")
		return
	}
	verified, err := verifySAMLRedirect(r.URL.RawQuery, cert)
	if err != nil {
		fail(w, r, 400, "invalid_saml_request")
		return
	}
	force, status, issued, err := validateSAMLRequest(request, config, b.Config.Issuer+"/saml/sso", now)
	if err != nil {
		fail(w, r, 400, "invalid_saml_request")
		return
	}
	browser := b.browser(w, r)
	st, err := b.registerSAMLRequest(config, request.Id, hash(browser), verified.Get("RelayState"), force, status, now, issued)
	if err != nil {
		samlHTTPError(w, r, err)
		return
	}
	b.samlIssue(w, r, st.ID, hash(browser))
}

func (b *Server) samlLogin(w http.ResponseWriter, r *http.Request) {
	if b.limited(r, "saml") {
		fail(w, r, 429, "rate_limited")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	cookie, e := r.Cookie("burrow_browser")
	if err != nil || e != nil || len(r.URL.RequestURI()) > samlMaxQuery || len(q) != 1 || len(q["requestId"]) != 1 || q.Get("requestId") == "" {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	b.samlIssue(w, r, q.Get("requestId"), hash(cookie.Value))
}

var samlForm = template.Must(template.New("saml-post").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Single sign-on</title></head><body><form method="post" action="{{.ACS}}"><input type="hidden" name="SAMLResponse" value="{{.Response}}"><input type="hidden" name="RelayState" value="{{.Relay}}"><button type="submit">Continue / 继续</button></form><script nonce="{{.Nonce}}">document.forms[0].submit()</script></body></html>`))

func (b *Server) samlIssue(w http.ResponseWriter, r *http.Request, id, browserHash string) {
	var body []byte
	var acs, nonce string
	interactive := false
	err := b.authenticationTx(func(tx *gorm.DB) error {
		now := time.Now()
		st, app, config, err := b.samlTransactionDB(tx, id, browserHash, now)
		if err != nil {
			return err
		}
		status := st.FailureStatus
		var user User
		var session Session
		if status == "" {
			user, session, err = b.sessionDB(tx, r)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, http.ErrNoCookie) || err.Error() == "invalid_authentication_method" {
					interactive = true
					return nil
				}
				return err
			}
			if !session.UTCTimestamps || session.AuthTime.After(now) || (st.ForceAuthn && (st.SessionID != session.ID || st.UserID != user.ID || session.AuthTime.Before(st.ReceivedAt))) {
				interactive = true
				return nil
			}
			p, admin, err := permissions(tx, user)
			if err != nil {
				return err
			}
			if !admin && !contains(p, "app:"+app.ID+":login") {
				status = "RequestDenied"
			}
		}
		key, cert, err := b.samlSigningKey(tx, now)
		if err != nil {
			return err
		}
		input := samlResponseInput{Issuer: b.samlEntityID(), Audience: config.EntityID, ACSURL: config.ACSURL, RequestID: st.SPRequestID, User: user, AuthTime: session.AuthTime, IssuedAt: now, SessionExpiresAt: session.ExpiresAt}
		var raw []byte
		if status == "" {
			raw, err = buildSAMLResponse(input, key, cert)
		} else {
			raw, err = buildSAMLFailure(input, status, key, cert)
		}
		if err != nil {
			return err
		}
		var buffered bytes.Buffer
		nonce = random(24)
		acs = config.ACSURL
		if err := samlForm.Execute(&buffered, struct{ ACS, Response, Relay, Nonce string }{acs, base64.StdEncoding.EncodeToString(raw), st.RelayState, nonce}); err != nil {
			return err
		}
		if err := consumeSAMLTransaction(tx, st, config, now, user.ID, status == "", requestID(r)); err != nil {
			return err
		}
		body = buffered.Bytes()
		return nil
	})
	if err != nil {
		samlHTTPError(w, r, err)
		return
	}
	if interactive {
		http.Redirect(w, r, "/login?requestType=saml&requestId="+url.QueryEscape(id), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	target, _ := url.Parse(acs)
	// CSP source expressions do not include query strings. Escape directive
	// separators in the registered path; the HTML action retains the exact ACS.
	source := target.Scheme + "://" + target.Host + target.EscapedPath()
	source = strings.NewReplacer(";", "%3B", "'", "%27", "\"", "%22").Replace(source)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action "+source+"; script-src 'nonce-"+nonce+"'")
	w.Write(body)
}

func samlHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	if strings.HasPrefix(err.Error(), "invalid_") {
		fail(w, r, 400, "invalid_transaction")
		return
	}
	fail(w, r, 503, "unavailable")
}
