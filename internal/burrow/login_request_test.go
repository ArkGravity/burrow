package burrow

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLoginRequestURL(t *testing.T) {
	for _, tc := range []struct {
		kind, id, want string
		bad            bool
	}{
		{"", "", "/", false}, {"", "old", "/oidc/login?requestId=old", false}, {"saml", "new", "/saml/login?requestId=new", false}, {"saml", "", "", true}, {"https://example.test", "new", "", true}, {"saml", "a&b", "/saml/login?requestId=a%26b", false},
	} {
		got, err := loginRequestURL(LoginRequestRef{Type: tc.kind, ID: tc.id})
		if (err != nil) != tc.bad || got != tc.want {
			t.Fatalf("kind=%q id=%q got=%q err=%v", tc.kind, tc.id, got, err)
		}
	}
}

func testSAMLContinuation(t *testing.T, b *Server, c *browser, id string) SAMLTransaction {
	t.Helper()
	a := Application{ID: random(18), Protocol: "saml", Name: "Jenkins", ClientID: random(18), ClientType: "saml", Enabled: true}
	if err := b.DB.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	config := SAMLApplication{ApplicationID: a.ID, EntityID: "https://jenkins.test/" + a.ID, ACSURL: "https://jenkins.test/acs", SigningCertificate: "test", MetadataXML: "test", ConfigVersion: 1}
	if err := b.DB.Create(&config).Error; err != nil {
		t.Fatal(err)
	}
	c.init(t)
	st := SAMLTransaction{ID: id, ApplicationID: a.ID, SPRequestID: "_" + id, BrowserHash: hash(c.cookies["burrow_browser"].Value), ACSURL: config.ACSURL, ConfigVersion: 1, ForceAuthn: true, ReceivedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if err := b.DB.Create(&st).Error; err != nil {
		t.Fatal(err)
	}
	return st
}

func TestSAMLLoginContinuation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			b, _, user := testServer(t, driver)
			c := newBrowser(b)
			st := testSAMLContinuation(t, b, c, "saml-request")
			b.DB.Model(&User{}).Where("id = ?", user.ID).Update("must_change_password", true)
			w := c.request("POST", "/api/v1/auth/login", map[string]string{"username": user.Username, "password": testPassword + "-changed", "requestId": st.ID, "requestType": "saml"}, true)
			if w.Code != 200 {
				t.Fatalf("SAML login %d %s", w.Code, w.Body.String())
			}
			var state map[string]any
			json.Unmarshal(w.Body.Bytes(), &state)
			if state["requestType"] != "saml" || state["step"] != "verify" {
				t.Fatal("continuation type lost", state)
			}
			c.verifyMFA(t)
			w = c.request("POST", "/api/v1/auth/login/password", map[string]string{"password": testPassword + "-saml"}, true)
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			json.Unmarshal(w.Body.Bytes(), &state)
			if state["redirect"] != "/saml/login?requestId="+st.ID {
				t.Fatal("incorrect protocol redirect", state)
			}
			var current SAMLTransaction
			b.DB.First(&current, "id = ?", st.ID)
			_, session, err := b.session(cookieRequest(c))
			if err != nil || current.UserID != user.ID || current.SessionID != session.ID {
				t.Fatal("fresh session not bound to SAML request", err)
			}
		})
	}
}
