<img src="web/public/logo.svg" alt="Burrow logo" width="64" height="64" />

# Burrow

[![CI](https://github.com/ArkGravity/burrow/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/ArkGravity/burrow/actions/workflows/ci.yml?query=branch%3Amain)
[![Release](https://img.shields.io/github/v/release/ArkGravity/burrow)](https://github.com/ArkGravity/burrow/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/ArkGravity/burrow?logo=go)](go.mod)
[![License](https://img.shields.io/github/license/ArkGravity/burrow)](LICENSE)

English · [简体中文](README_zh.md)

A lightweight OpenID Connect and SAML identity provider for small teams and self-hosted
services. Burrow centralizes accounts, shared SSO and application access in a
single-organization, single-instance service. Its Go backend and React UI ship
as one binary, or run with PostgreSQL through Docker Compose.

[Releases](https://github.com/ArkGravity/burrow/releases) ·
[Installation](docs/installation.md) · [Documentation](docs/README.md) ·
[Changelog](CHANGELOG.md)

## Features and scope

- **Shared SSO:** connect Web and SPA applications through OIDC Authorization Code, with PKCE S256 required by default.
- **SAML IdP:** signed SP-initiated Redirect requests and double-signed POST responses, with a manually configured [Jenkins example](examples/local-sso/jenkins/README.md).
- **Password authentication:** temporary-password changes and optional TOTP MFA; MFA is disabled by default and applies to all accounts when enabled.
- **Access management:** users, groups, custom roles, permissions, audited changes and a portal of authorized applications.
- **Localized UI:** English and Simplified Chinese, with light, dark and system themes.
- **Simple deployment:** one binary with an embedded UI; PostgreSQL in production and SQLite for development and tests, without Redis or a queue.

Inspired by Casdoor and independently implemented, Burrow focuses on password
login, downstream OIDC and the agreed SAML subset. Upstream identity federation, Refresh Tokens and
cross-application logout are outside its current scope. Applications manage their
own sessions and business permissions; see [protocol boundaries](docs/development/oidc-adapter.md).
SAML scope and current interoperability evidence are documented [separately](docs/testing/saml-jenkins.md).

## Screenshots

The application portal groups authorized services by protocol: Grafana and
Nightingale under OIDC, and Jenkins under SAML.

![Burrow application portal grouped by OIDC and SAML, showing Grafana, Nightingale and Jenkins](docs/screenshots/overview.jpg)

<details>
<summary>Application, role, group and user management</summary>

![Create a Nightingale OIDC application](docs/screenshots/create-application-oidc.jpg)
![Create a Jenkins SAML application](docs/screenshots/create-application-saml.jpg)
![Configure login permissions for all three applications](docs/screenshots/create-roles.jpg)
![Create a group with the application access role](docs/screenshots/create-groups.jpg)
![Create a user with a group assignment](docs/screenshots/create-users.jpg)

</details>

[Capture details and verification scope](docs/screenshots/README.md).

## Install and sign in

**Docker Compose is recommended.** Published images and Linux amd64/arm64 binaries
include the frontend; Go and Bun are not needed to run them. For Compose, install
Docker Engine and Compose v2, then download the deployment archive from
[Releases](https://github.com/ArkGravity/burrow/releases). This example pins a release:

```bash
BURROW_RELEASE=v0.1.4
tar -xzf "burrow_${BURROW_RELEASE}_deploy.tar.gz"
cd "burrow_${BURROW_RELEASE}_deploy"
cp .env.example .env
```

Edit `.env` before starting: set production mode and your HTTPS issuer, use
independent master-key and administrator-password values, and set the PostgreSQL
password consistently in `POSTGRES_PASSWORD` and `BURROW_DB_DSN`. Keep the image
version supplied by the archive and configure your TLS reverse proxy.
[Installation instructions](docs/installation.md) cover secret generation and proxy settings.

```bash
docker compose --profile local-db config --quiet
docker compose --profile local-db pull
docker compose --profile local-db up -d --no-build --pull never
```

Open your configured HTTPS issuer, sign in as `admin` with your configured
administrator password, and change the temporary password. Compose runs migration
and seed before the server. For binary installation, MFA and upgrades, follow the
[installation guide](docs/installation.md).

## Connect applications

1. Create users, groups and ordinary roles.
2. Choose OIDC or SAML when registering an application. For OIDC, configure a Web or SPA client with exact callback URLs and any required browser origins. For SAML, import the service provider metadata and configure its trust in Burrow's IdP metadata. Set the application's login URL for portal access.
3. Add its `app:<id>:login` permission to a role and assign the role to users or groups.
4. Users see authorized applications grouped by OIDC or SAML in their portal; each application starts its own authentication request.

Administrator is the only built-in role. New users have no roles by default;
profiles and the portal only require authentication. Application access requires
an explicit role grant unless the user is an administrator.

See [Web / SPA examples](examples/README.md) and
[Grafana / Nightingale / Harbor setup](examples/local-sso/README.md), plus the
[Jenkins SAML guide](examples/local-sso/jenkins/README.md).

## Development and contributions

Install Go, Bun, Make and a C compiler using the versions in
[the development guide](docs/development/getting-started.md). From a source checkout:

```bash
make deps
make migrate
make seed
make run       # backend: http://localhost:8080
```

Run `make web-dev` in another terminal and open http://localhost:5173.
The [development guide](docs/development/getting-started.md) covers local credentials,
configuration, builds and checks. See [Contributing](CONTRIBUTING.md) for issues and pull requests,
and [Security](SECURITY.md) for private vulnerability reports.

## Documentation and license

- [Documentation index](docs/README.md)
- [Configuration and MFA policy](docs/development/configuration.md)
- [Backup, upgrades and MFA recovery](docs/operations/recovery.md)
- [Verification scope and checkpoints](docs/testing/oidc-conformance.md)

Burrow is released under the [MIT license](LICENSE).
