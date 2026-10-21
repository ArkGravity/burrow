# Product screenshots

The current JPEGs were captured and supplied by the maintainer on October 21, 2026,
during manual browser acceptance of the [local SSO example](../../examples/local-sso/README.md)
at `http://sso.yakir.top`. They show the UI in English and dark mode. The example
runs the current development branch with MFA disabled by default.

- [`overview.jpg`](overview.jpg): the `logic` user's application portal with
  Grafana and Nightingale (N9E) grouped under OIDC, and Jenkins under SAML.
- [`create-application-oidc.jpg`](create-application-oidc.jpg): the Nightingale
  OIDC Web application creation form.
- [`create-application-saml.jpg`](create-application-saml.jpg): the Jenkins SAML
  application creation form, including its login URL, icon URL and metadata import.
- [`create-roles.jpg`](create-roles.jpg): the role creation form with Grafana,
  Nightingale and Jenkins login permissions selected.
- [`create-groups.jpg`](create-groups.jpg): the group creation form with the
  application access role selected.
- [`create-users.jpg`](create-users.jpg): the `logic` user creation form with a
  group assignment.

The older `user-app-overview.jpg`, `create-group.jpg` and `create-user.jpg` files
remain as historical captures from October 9, 2026; the project READMEs use the
current set above. The former `create-application.jpg` has been replaced by the
separate OIDC and SAML captures.

The images show the product viewport without browser toolbars or bookmarks.
Passwords and client secrets are masked; no readable tokens or authenticator
setup codes are visible. The supplied images are used without modification.

The maintainer reported successful local integration and login with Grafana and
Nightingale through OIDC and Jenkins through SAML, then confirmed completion of
the portal and management UI refinements and supplied these updated screenshots.
The earlier October 9 checkpoint covered Burrow management, user login and portal
access only. These user reports are recorded separately from automated checks;
neither establishes production validation or official protocol certification.
See the [verification record](../testing/oidc-conformance.md) and
[SAML / Jenkins acceptance record](../testing/saml-jenkins.md).
