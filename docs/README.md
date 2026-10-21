# Documentation

Start with the root [README](../README.md) for the product overview, installation
and application onboarding.

A [Simplified Chinese introduction and setup guide](../README_zh.md) is also available.

## Getting started

- [Installation and first login](installation.md) · [简体中文](installation_zh.md)
- [Local development, builds and checks](development/getting-started.md)
- [Contributing](../CONTRIBUTING.md)
- [Private vulnerability reporting](../SECURITY.md)
- [v1.0.0 release notes and upgrade requirements](releases/v1.0.0.md)
- [Independent Web and SPA clients](../examples/README.md)

## Development

- [Project conventions](../AGENTS.md)
- [Default configuration](../configs/config.yaml)
- [Configuration loading, overrides and secrets](development/configuration.md)
- [Dependencies and implementation layout](development/dependencies.md)
- [CI checks and image publishing](development/ci.md)
- [Version release preparation and publication](development/releases.md)
- [Release installation template](releases/INSTALL.md) (rendered for each release)
- [Seed and default role boundaries](development/seed.md)
- [OIDC adapter boundaries](development/oidc-adapter.md)
- [SAML IdP adapter boundaries](development/saml-adapter.md)
- [Mandatory MFA and account recovery](development/mfa-proposal.md)

## Operations and verification

- [Backup, recovery, upgrades and signing-key rotation](operations/recovery.md)
- [Grafana OIDC integration and acceptance](operations/grafana.md)
- [Jenkins SAML configuration and recovery](operations/jenkins.md)
- [SAML and Jenkins verification evidence](testing/saml-jenkins.md)
- [Local Docker Burrow, Grafana, Nightingale, Harbor, Jenkins and Nginx example](../examples/local-sso/README.md)
- [Protocol interoperability and regression verification](testing/oidc-conformance.md)
- [CI workflow](../.github/workflows/ci.yml)

## Design history

- [Approved first-release specification](superpowers/specs/2026-09-24-burrow-idp-design.md)
- [Implementation plan and execution record](superpowers/plans/2026-09-24-burrow-idp.md)

Historical plans describe decisions made during the initial implementation.
Use the current README and configuration reference for executable commands;
older environment-only setup instructions have been replaced by YAML defaults.

Upstream Provider support was removed in schema v4. The older design and plan
retain their historical OP/RP scope; current authentication uses Burrow passwords.
See [upgrade guidance](operations/recovery.md#upgrade-to-password-only-authentication).
