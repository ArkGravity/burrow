package burrow

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"gorm.io/gorm"
)

const samlMetadataNS = "urn:oasis:names:tc:SAML:2.0:metadata"
const samlDSigNS = "http://www.w3.org/2000/09/xmldsig#"
const samlPOSTBinding = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
const samlRedirectBinding = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect"

func samlChildren(parent *etree.Element, namespace, name string) []*etree.Element {
	var children []*etree.Element
	for _, child := range parent.ChildElements() {
		if child.Tag == name && child.NamespaceURI() == namespace {
			children = append(children, child)
		}
	}
	return children
}

func (b *Server) parseSAMLMetadata(raw string, now time.Time) (*SAMLApplication, error) {
	bad := invalid("request")
	if validateSAMLXML([]byte(raw), samlMetadataNS, "EntityDescriptor") != nil {
		return nil, bad
	}
	doc := etree.NewDocument()
	if doc.ReadFromString(raw) != nil {
		return nil, bad
	}
	root := doc.Root()
	// A single explicit SP and a single POST ACS form the imported trust boundary.
	sps := samlChildren(root, samlMetadataNS, "SPSSODescriptor")
	if len(sps) != 1 || len(samlChildren(root, samlMetadataNS, "EntityDescriptor")) != 0 {
		return nil, bad
	}
	entity := root.SelectAttrValue("entityID", "")
	u, err := url.Parse(entity)
	if err != nil || entity == "" || len(entity) > 1024 || u.Scheme == "" || strings.TrimSpace(entity) != entity {
		return nil, bad
	}
	sp := sps[0]
	if !contains(strings.Fields(sp.SelectAttrValue("protocolSupportEnumeration", "")), samlProtocolNS) {
		return nil, bad
	}
	for _, element := range []*etree.Element{root, sp} {
		if value := element.SelectAttrValue("validUntil", ""); value != "" {
			expires, err := time.Parse(time.RFC3339Nano, value)
			if err != nil || !now.Before(expires) {
				return nil, bad
			}
		}
	}
	var acs []*etree.Element
	indexes := map[string]bool{}
	for _, endpoint := range samlChildren(sp, samlMetadataNS, "AssertionConsumerService") {
		index := endpoint.SelectAttrValue("index", "")
		value, err := strconv.ParseUint(index, 10, 16)
		canonical := strconv.FormatUint(value, 10)
		if err != nil || indexes[canonical] {
			return nil, bad
		}
		indexes[canonical] = true
		if endpoint.SelectAttrValue("Binding", "") == samlPOSTBinding {
			acs = append(acs, endpoint)
		}
	}
	if len(acs) != 1 {
		return nil, bad
	}
	acsURL := acs[0].SelectAttrValue("Location", "")
	if b.validateURL(acsURL, false) != nil || acs[0].SelectAttrValue("ResponseLocation", "") != "" {
		return nil, bad
	}
	var certs []*x509.Certificate
	for _, descriptor := range samlChildren(sp, samlMetadataNS, "KeyDescriptor") {
		use := descriptor.SelectAttrValue("use", "")
		if use == "encryption" {
			continue
		}
		if use != "" && use != "signing" {
			return nil, bad
		}
		infos := samlChildren(descriptor, samlDSigNS, "KeyInfo")
		if len(infos) != 1 {
			return nil, bad
		}
		data := samlChildren(infos[0], samlDSigNS, "X509Data")
		if len(data) != 1 || len(samlChildren(infos[0], samlDSigNS, "RetrievalMethod")) > 0 {
			return nil, bad
		}
		certNodes := samlChildren(data[0], samlDSigNS, "X509Certificate")
		if len(certNodes) != 1 {
			return nil, bad
		}
		der, err := base64.StdEncoding.Strict().DecodeString(strings.Join(strings.Fields(certNodes[0].Text()), ""))
		if err != nil {
			return nil, bad
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, bad
		}
		key, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok || key.N.BitLen() < 2048 || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || (cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageDigitalSignature == 0) {
			return nil, bad
		}
		certs = append(certs, cert)
	}
	if len(certs) != 1 {
		return nil, bad
	}
	return &SAMLApplication{EntityID: entity, ACSURL: acsURL, SigningCertificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs[0].Raw})), MetadataXML: raw, ConfigVersion: 1}, nil
}

// Parse untrusted XML before taking the administrative mutation lock.
func (b *Server) samlApplicationInput(in map[string]json.RawMessage) (*SAMLApplication, error) {
	raw, present := in["saml"]
	if !present {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 1 {
		return nil, invalid("request")
	}
	var metadata string
	if json.Unmarshal(fields["metadataXml"], &metadata) != nil || metadata == "" {
		return nil, invalid("request")
	}
	return b.parseSAMLMetadata(metadata, time.Now())
}

func (b *Server) saveSAMLApplication(tx *gorm.DB, v *Application, in map[string]json.RawMessage, parsed *SAMLApplication, creating, admin bool) error {
	for _, field := range []string{"clientId", "clientType", "clientSecret", "redirectUris", "postLogoutRedirectUris", "origins", "allowWithoutPkce"} {
		if _, present := in[field]; present {
			return invalid("request")
		}
	}
	data, _ := json.Marshal(in)
	if json.Unmarshal(data, v) != nil || strings.TrimSpace(v.Name) == "" {
		return invalid("request")
	}
	v.Protocol = "saml"
	v.ClientType = "saml"
	v.SecretHash = ""
	v.SAML = nil
	if v.LoginURL != "" && b.validateURL(v.LoginURL, false) != nil {
		return invalid("url")
	}
	if v.Enabled {
		if _, _, err := b.samlSigningKey(tx, time.Now()); err != nil {
			return errors.New("unavailable")
		}
	}
	var config SAMLApplication
	if creating {
		if parsed == nil {
			return invalid("request")
		}
	} else if err := tx.First(&config, "application_id = ?", v.ID).Error; err != nil {
		return err
	}
	if err := tx.Save(v).Error; err != nil {
		return err
	}
	if parsed != nil {
		version := int64(1)
		if !creating {
			version = config.ConfigVersion
			if parsed.MetadataXML != config.MetadataXML {
				version++
			}
		}
		config = *parsed
		config.ApplicationID = v.ID
		config.ConfigVersion = version
		if err := tx.Save(&config).Error; err != nil {
			return err
		}
		if !creating {
			if err := tx.Where("application_id = ? AND config_version <> ?", v.ID, version).Delete(&SAMLTransaction{}).Error; err != nil {
				return err
			}
		}
	}
	if creating {
		pid := "app:" + v.ID + ":login"
		if err := tx.Create(&Permission{ID: pid, Name: pid, ApplicationID: v.ID, Description: "Login to " + v.Name}).Error; err != nil {
			return err
		}
	}
	if err := b.assign(tx, "applications", v.ID, in, admin); err != nil {
		return err
	}
	b.hydrateApp(tx, v)
	v.SAML = &config
	return nil
}

// Internal compatibility client IDs are not public SAML identities.
func (a Application) MarshalJSON() ([]byte, error) {
	type plain Application
	v := plain(a)
	if a.Protocol == "saml" {
		v.ClientID = ""
	}
	return json.Marshal(v)
}
