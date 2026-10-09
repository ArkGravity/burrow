# CI and image publishing

[Burrow CI](../../.github/workflows/ci.yml) follows the parallel verification and
main-branch image publishing structure of
[Optimus CI](https://github.com/ArkGravity/optimus/blob/main/.github/workflows/ci.yaml).

## Triggers and checks

Pull requests, pushes to `dev` and manual `workflow_dispatch` runs execute full
regression. A `plan` job chooses coverage before the frontend and backend checks;
image builds run in parallel with it. Full regression includes:

- Frontend typecheck, unit tests and production build with Bun 1.4.2 and frozen
  dependencies.
- Go static analysis and release gate, packaging and image-publication regression
  tests and CI verification-reuse/aggregation tests, using the Go version in
  `go.mod` and Python's standard library.
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
Main runs also smoke-test each actual image in production mode against a separate
PostgreSQL service: migration, repeated seed, startup, readiness, healthcheck and
embedded UI. They save each image as an Actions artifact for publication without
rebuilding. The Dockerfile declares version/commit/build-time arguments immediately
before compilation so metadata changes do not invalidate dependency-download layers.

## Main regression reuse and merge protection

For a push to `main`, `plan` finds a PR merged into this exact commit and a
successful `pull_request` run of this repository's `ci.yml` for that PR's final
head SHA. After full PR regression, `verify` uploads `ci-verification`, containing
the actual checked-out Git tree, PR number, repository, head SHA and run ID/attempt.
The main plan downloads that record and checks every field, including equality
between the tested tree and main's tree. This supports squash merges without
confusing the source-head SHA with the tested merge result.

Only a matching record allows main to skip frontend checks, static analysis,
script tests, backend race tests and browser regression. Compose validation,
both native image builds, production image smoke tests and publishing still run.
Direct pushes, changed merge results, missing/expired records, failed PR CI and
GitHub API/download errors fall back to full regression. Old PR runs without the
record also take the full path. PR records are retained for 30 days; an expired
record costs a full regression run rather than bypassing verification. Manual
runs always request full regression, including on `main`.

Configure `main` branch protection to require PRs and the `verify` check from the
GitHub Actions app, with **Require branches to be up to date before merging** and
administrator enforcement enabled. No additional approving reviewer is required;
this permits a single maintainer to merge after CI. Force pushes and branch
deletion remain disabled. Repository protection is a GitHub setting, not an effect
of the workflow YAML. If merge queue is introduced later, add `merge_group`
coverage before making this check required for the queue.

The `verify` job requires `plan` and both image builds to succeed. Full regression
requires every regression job to succeed; the reuse path requires precisely those
four jobs to be skipped, and is accepted only for a main push. Failure,
cancellation, unexpected skips and missing coverage outputs block publication.
Its check name remains `verify` for branch protection.
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

After `verify` accepts the selected coverage, a push or manual run on `main` downloads
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
The publisher binary is cached by OS, runner architecture and pinned crane version;
cache hits skip Go setup and tool compilation. Main and release publishers use the
same key, while image builds retain separate architecture-specific cache scopes.

Saved image artifacts are retained for seven days. Publishing failures may leave
untagged platform manifests or an index in only one registry; retry the publishing job while
artifacts exist. If they have expired, rerun the complete workflow.
Rebuilding jobs replace their own named artifacts on retry. PR verification records
also replace the previous attempt's record, and main validates the run attempt.

Set `BURROW_IMAGE` in the deployment's untracked `.env.prod` to the desired tag
or digest, then follow the [root deployment instructions](../../README.md#containers-and-deployment)
to pull the image and run migration, seed and server. CI publishes images;
deployments remain explicit. Preserve the database and original master key when
upgrading.
