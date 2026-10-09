# Changelog

## v0.1.3 (2026-10-09)

Published as [Burrow v0.1.3](https://github.com/ArkGravity/burrow/releases/tag/v0.1.3).

- Reuse successful PR regression on main only when the recorded tested Git tree
  matches exactly. Missing or invalid evidence falls back to full regression.
  Main still builds, smoke-tests and publishes both native architecture images.
- Improve release build cache reuse, cache the pinned image publisher and retain
  tested image artifacts for seven days. Packaged-binary browser tests and
  PostgreSQL container smoke tests remain required on both architectures.
- Simplify Users actions: use password and MFA reset icons with translated
  tooltips, and remove the standalone Revoke sessions action and API. Password
  reset, MFA reset, logout and account changes retain their session invalidation.

See [v0.1.3 release notes](docs/releases/v0.1.3.md). Schema v5 and the global MFA
policy are unchanged; MFA remains disabled by default and requires explicit
configuration when desired.

## v0.1.2 (2026-10-09)

Published as [Burrow v0.1.2](https://github.com/ArkGravity/burrow/releases/tag/v0.1.2).

- Add a global MFA policy through `security.mfa_enabled` and
  `BURROW_MFA_ENABLED`, disabled by default. Enable it explicitly to require
  TOTP for every account. Preserve existing bindings and forced temporary
  password changes; OIDC authentication claims reflect the actual login.
- Remove the trailing slash from the login brand and log out directly from the
  signed-in shell with one click.
- Explain password policy errors and validate the 12–256 UTF-8 byte limit in
  user creation, password reset and password change forms.
- Refresh both READMEs with maintainer-supplied local acceptance screenshots
  and add application icon examples for Grafana and Nightingale.

- Publish only a unified multi-platform image tag per commit or version in GHCR
  and Docker Hub. Upload tested amd64 and arm64 images by digest without creating
  architecture-suffixed tags.

See [v0.1.2 release notes](docs/releases/v0.1.2.md) for installation and the MFA
upgrade setting. Schema v5 is unchanged.

## v0.1.1 (2026-10-07)

Published as [Burrow v0.1.1](https://github.com/ArkGravity/burrow/releases/tag/v0.1.1).

- Native Linux amd64 and arm64 CI tests and image builds, with matching
  multi-platform images in GHCR and Docker Hub.
- Release preparation tests both architecture binaries and containers, reuses
  saved images for publication, and provides separate binary archives plus a
  shared deployment archive, image digests and checksums. Both native CI and
  release preparation jobs passed; published v0.1.0 remains amd64-only.

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
