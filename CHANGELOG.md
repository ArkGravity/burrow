# Changelog

## Unreleased

- Native Linux amd64 and arm64 CI tests and image builds, with matching
  multi-platform images in GHCR and Docker Hub.
- Release preparation tests both architecture binaries and containers, reuses
  saved images for publication, and provides separate binary archives plus a
  shared deployment archive, image digests and checksums. Remote verification
  of this workflow is pending; published v0.1.0 remains amd64-only.

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

See [release notes](docs/releases/v0.1.0.md), [installation](https://github.com/ArkGravity/burrow/releases/download/v0.1.0/INSTALL.md)
and [upgrade/recovery guidance](docs/operations/recovery.md).
