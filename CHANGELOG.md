# Changelog

## v0.1.0 (2026-10-02)

First release of Burrow, a single-organization OpenID Connect identity provider.
Published as [Burrow v0.1.0](https://github.com/ArkGravity/burrow/releases/tag/v0.1.0).

- Password authentication and mandatory TOTP, shared SSO, forced temporary
  password changes and administrator/operator MFA recovery.
- Users, groups, roles, permissions, application configuration, personal
  profiles and application portals, with transactional auditing and RBAC.
- Web/SPA Authorization Code with PKCE S256, persistent signing keys and
  rotation, exact redirects/origins and current-access revalidation.
- English/Simplified Chinese and light/dark/system themes.
- PostgreSQL production and SQLite development/test support; schema v5.
- MIT license, version reporting and a tag-driven release workflow producing
  a draft, tested Linux amd64 binaries, versioned container images, deployment
  archives and checksums.

See [release notes](docs/releases/v0.1.0.md), [installation](docs/releases/INSTALL.md)
and [upgrade/recovery guidance](docs/operations/recovery.md).
