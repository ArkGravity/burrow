# Version releases

The [release workflow](../../.github/workflows/release.yml) prepares a GitHub
Release draft and fixed-version images. The existing [CI](ci.md) continues to
verify pull requests and publish development images from `main`.

## Preparing a version

1. Add `docs/releases/vX.Y.Z.md`, update the changelog and installation examples,
   and keep manifest versions aligned where applicable. Select a new stable
   version: the published `v0.1.0` is amd64-only and must not be replaced.
   `docs/releases/INSTALL.md` is a template; packaging replaces `@VERSION@`
   with the selected tag in every archive and the attached installation guide.
2. Merge release preparation into `main` and wait for its complete push CI to
   succeed. The workflow checks the exact commit, main ancestry and API results;
   PR CI or a different commit's green run cannot substitute.
3. Create and push an annotated tag at that tested commit:

   ```bash
   git tag -a vX.Y.Z <tested-main-commit> -m "Burrow vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. Wait for both native preparation jobs: Linux amd64 on `ubuntu-24.04` and
   Linux arm64 on `ubuntu-24.04-arm`. Each builds and loads a single-platform
   image with embedded UI and version/commit/build-time metadata. Build time is
   the tagged commit's time. Each extracts that exact binary, checks the ELF
   architecture, packages and checksums it, then runs the existing SQLite/MFA/OIDC
   browser suite against the extracted archive binary. Each dedicated PostgreSQL
   service also verifies migration, repeated seed, production container startup,
   readiness, healthcheck and embedded UI. Both jobs must succeed.
5. The draft job verifies both downloaded archive checksums, creates the shared
   deployment archive and renders the installation guide. It downloads the tested
   images without rebuilding, validates architecture/revision, and pushes
   `vX.Y.Z-amd64` and `vX.Y.Z-arm64` tags to GHCR and Docker Hub. It assembles
   `vX.Y.Z` multi-platform indexes from immutable digests, requiring exactly the
   two tested platforms, anonymous index access and matching platform/index
   digests across registries. It attaches the six files listed below to a draft
   Release. Review these results and publish the draft:

   ```bash
   gh release edit vX.Y.Z --draft=false --latest
   ```

Publishing the draft is a separate delivery action. Do not publish it while
the workflow is running or incomplete. GitHub automatically adds source archive
links; these are separate from the tested binary and deployment attachments.

## Attachments and image identity

- `burrow_vX.Y.Z_linux_amd64.tar.gz`: tested x86-64 binary with embedded frontend.
- `burrow_vX.Y.Z_linux_arm64.tar.gz`: tested AArch64 binary with embedded frontend.
- `burrow_vX.Y.Z_deploy.tar.gz`: one shared Compose deployment archive.
- `INSTALL.md`: rendered instructions for the selected version and both architectures.
- `IMAGES.txt`: two multi-platform tag/index digests, followed by four
  architecture-specific tag/manifest digests, one pair per registry.
- `SHA256SUMS`: checksums for the other five attachments.

The version tag and index digest select the runtime architecture automatically.
Architecture-specific tags/digests can be used when a fixed platform is required.
The saved images have no provenance or SBOM attestations; the assembled indexes
contain exactly `linux/amd64` and `linux/arm64`. Cache scopes, saved image artifacts,
binary artifacts, archive checksum files and failed browser reports include the
architecture to prevent collisions. Shared attachments are generated only after
both architecture jobs succeed.

## Permissions and failures

The validation job uses `contents: read` and `actions: read`. Only the draft
job receives `contents: write` and `packages: write`, plus `actions: read` to
recheck CI eligibility; registry credentials follow
the existing [CI configuration](ci.md#registry-configuration). GHCR package and
Docker Hub repository must allow public pulls. Package visibility is a registry
setting, not a workflow field.

Non-tag refs, invalid/unstable version tags, commits outside `main`, missing or
failed exact-commit CI, API errors and already published Releases are rejected.
A failed build/test produces no publication. Registry/network failures can leave
architecture tags or one multi-platform index published without a complete draft: fix the failure and
rerun failed jobs to reuse the tested image. Intermediate Actions image artifacts
are retained for one day and release assets for seven days. If artifacts have
expired, rerun the whole workflow while the version is still unpublished.

For a tag pushed before main CI completes, rerun after CI succeeds. Manual
dispatch must select the existing tag, not `main`. Draft assets may be replaced
on retry; published versions must receive a new patch version instead. The
workflow intentionally does not publish `latest` or moving minor-version tags.

After the first successful publication, change the changelog's release-preparation
label to the actual publication date and record the verified release URL and
checks. Never label an unexecuted workflow as verified.

## Local checks

```bash
python3 -m unittest discover -s scripts/release -p 'test_*.py'
make build VERSION=vX.Y.Z
./bin/burrow version
make web-install
BURROW_E2E_BINARY="$PWD/bin/burrow" make test-e2e
make compose-config COMPOSE_ENV=.env.example
```

The downloadable Linux binaries come from their respective Debian bookworm
container builds and require glibc 2.36+. Do not use a locally built macOS binary
as a Linux asset or disable CGO: SQLite depends on it. The workflow must complete
on both native runners before describing a particular version as verified for
both architectures. The historical `v0.1.0` release remains amd64-only.
