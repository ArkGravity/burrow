# Product screenshots

These JPEGs were captured and supplied by the maintainer on October 9, 2026,
during manual browser acceptance of the [local SSO example](../../examples/local-sso/README.md)
at `http://sso.yakir.top`. They show the UI in English and dark mode. The example
runs the current development branch with MFA disabled by default.

- `user-app-overview.jpg`: the `logic` user's application portal with assigned
  Grafana and Nightingale (N9E) applications.
- `create-application.jpg`: the Nightingale Web application creation form.
- `create-roles.jpg`: the role editing form with Grafana and Nightingale login
  permissions selected.
- `create-group.jpg`: the group creation form with a shared role selected.
- `create-user.jpg`: the `logic` user creation form with role and group assignments.

The images show the product viewport without browser toolbars or bookmarks.
Passwords and client secrets are masked; no readable tokens or authenticator
setup codes are visible. The supplied images are used without modification.

The maintainer reported successful administrator creation of applications, roles,
groups and users, followed by successful login and application portal access as
the new `logic` user. This checkpoint covers those Burrow flows; downstream SSO
login, production deployment and official OIDC certification were not part of
this reported acceptance. See the [verification record](../testing/oidc-conformance.md).
