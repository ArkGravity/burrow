# CI and image publishing

[Burrow CI](../../.github/workflows/ci.yml) follows the parallel verification and
main-branch image publishing structure of
[Optimus CI](https://github.com/ArkGravity/optimus/blob/main/.github/workflows/ci.yaml).

## Triggers and checks

Pushes to `main` and `dev`, all pull requests and manual `workflow_dispatch` runs
execute these jobs in parallel:

- Frontend typecheck, unit tests and production build with Bun 1.4.2 and frozen
  dependencies.
- Go static analysis and release gate, packaging and image-publication regression
  tests, using the Go version in `go.mod` and Python's standard library.
- Go race tests against SQLite and a dedicated PostgreSQL 17 service on both architectures.
- Chromium browser tests on both architectures with the independent Web and SPA OIDC clients. The
  runner installs Chromium's system dependencies; failed browser runs upload
  `web/test-results/` under architecture-specific artifact names.
- Quiet root Compose validation using `.env.example`, followed by a Dockerfile
  build on both architectures with Buildx and architecture-specific GitHub Actions caches.

The backend, browser and image jobs use native `ubuntu-24.04` (amd64) and
`ubuntu-24.04-arm` (arm64) runners. Both matrix legs must succeed; a failing leg
does not cancel the other leg. Frontend checks and static analysis run once.
The Dockerfile retains CGO and Debian bookworm for SQLite and glibc compatibility;
no emulation or cross-compiler is needed. Every image build is loaded locally.
Main runs save each image as an Actions artifact for publication without rebuilding.

The `verify` job collects all results and fails if any required job fails or is
cancelled or skipped. Its existing check name remains available for branch protection.
New runs cancel previous runs for the same event and ref except on `main`.

## Registry configuration

Configure the following under repository **Settings → Secrets and variables →
Actions** before enabling publishing on `main`:

| Setting               | Kind     | Value                                                                     |
| --------------------- | -------- | ------------------------------------------------------------------------- |
| `DOCKERHUB_TOKEN`     | Secret   | Docker Hub access token with write access to the target repository        |
| `DOCKERHUB_USERNAME`  | Variable | Docker Hub login; defaults to `logic3579`                                 |
| `DOCKERHUB_NAMESPACE` | Variable | Docker Hub user or organization owning the image; defaults to `logic3579` |

For compatibility with Optimus, `DOCKERHUB_USERNAME` can also be a secret; the
repository variable takes precedence. Create the `burrow` Docker Hub repository
in the selected namespace and ensure the login account can push to it.

GHCR uses the automatically supplied `GITHUB_TOKEN`. Only the publishing job
receives `packages: write`; verification jobs have `contents: read`. Repository
and organization policies must allow Actions to publish packages. If a GHCR
package already exists, grant this repository Actions access to that package.
For public pulls, set the GHCR package visibility to public in its package
settings.

Missing Docker Hub credentials fail the publishing job before registry login.
Credentials are never printed by the workflow. No registry login is performed
for pull requests or `dev` runs.

## Images and deployment

After every verification job succeeds, a push or manual run on `main` downloads
the saved amd64 and arm64 images and publishes a multi-platform index to both registries:

- `ghcr.io/arkgravity/burrow:main-<short-sha>`
- `docker.io/<DOCKERHUB_NAMESPACE>/burrow:main-<short-sha>`

The short SHA is the first seven characters of the workflow commit, matching
Optimus's tag convention. GHCR derives its image name from `github.repository`
and normalizes it to lowercase. The CI workflow does not publish `latest`, release
tags or images from pull requests and other branches. The separate
[release workflow](releases.md) prepares fixed version tags and Release drafts.
Docker selects `linux/amd64` or `linux/arm64` automatically from the same tag.
The publishing job installs pinned `crane` v0.22.1 to upload the tested image
archives by digest, then assembles indexes from those immutable digests. Only
`main-<short-sha>` receives a tag; no architecture-suffixed tags are created.
It validates saved-image architecture and revision, pushed image configuration,
exact index membership, anonymous index access and matching platform/index digests
across both registries. Provenance and SBOM attestations are disabled for the
saved single-platform images; the final index contains only the two runtime platforms.

Saved image artifacts are retained for one day. Publishing failures may leave
untagged platform manifests or an index in only one registry; retry the publishing job while
artifacts exist. If they have expired, rerun the complete workflow.

Set `BURROW_IMAGE` in the deployment's untracked `.env.prod` to the desired tag
or digest, then follow the [root deployment instructions](../../README.md#containers-and-deployment)
to pull the image and run migration, seed and server. CI publishes images;
deployments remain explicit. Preserve the database and original master key when
upgrading.
