# Documentation

Start with the root [README](../README.md) for local development, common Make
targets and Docker Compose deployment.

## Development

- [Project conventions](../AGENTS.md)
- [Default configuration](../configs/config.yaml)
- [Configuration loading, overrides and secrets](development/configuration.md)
- [Dependencies and implementation layout](development/dependencies.md)
- [Seed and default role boundaries](development/seed.md)
- [OIDC adapter boundaries](development/oidc-adapter.md)
- [Independent Web and SPA clients](../examples/README.md)

## Operations and verification

- [Backup, recovery, upgrades and signing-key rotation](operations/recovery.md)
- [Grafana OIDC integration and acceptance](operations/grafana.md)
- [Protocol interoperability and regression verification](testing/oidc-conformance.md)
- [CI workflow](../.github/workflows/ci.yml)

## Design history

- [Approved first-release specification](superpowers/specs/2026-09-24-burrow-idp-design.md)
- [Implementation plan and execution record](superpowers/plans/2026-09-24-burrow-idp.md)

Historical plans describe decisions made during the initial implementation.
Use the current README and configuration reference for executable commands;
older environment-only setup instructions have been replaced by YAML defaults.
