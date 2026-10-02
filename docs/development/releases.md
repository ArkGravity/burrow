# Version releases

The [release workflow](../../.github/workflows/release.yml) prepares a GitHub
Release draft and fixed-version images. The existing [CI](ci.md) continues to
verify pull requests and publish development images from `main`.

## Preparing a version

1. Add `docs/releases/vX.Y.Z.md`, update the changelog and installation examples,
   and keep manifest versions aligned where applicable. The initial version is
   `v0.1.0`; the frontend manifest already declares `0.1.0`.
2. Merge release preparation into `main` and wait for its complete push CI to
   succeed. The workflow checks the exact commit, main ancestry and API results;
   PR CI or a different commit's green run cannot substitute.
3. Create and push an annotated tag at that tested commit:

   ```bash
   git tag -a v0.1.0 <tested-main-commit> -m "Burrow v0.1.0"
   git push origin v0.1.0
   ```

4. Wait for Release to build and load a Linux amd64 image with embedded UI and
   version/commit/build-time metadata. Build time is the tagged commit's time.
   It extracts that exact binary, packages and checksums it, and runs the
   existing SQLite/MFA/OIDC browser suite against the extracted archive binary.
   A dedicated PostgreSQL service also verifies migration, repeated seed,
   production container startup, readiness, healthcheck and embedded UI.
5. The draft job downloads the tested image without rebuilding, pushes it to
   GHCR and Docker Hub, verifies anonymous manifest access and matching digests,
   and attaches both archives, installation instructions, image references and
   checksums to a draft Release. Review these results and publish the draft:

   ```bash
   gh release edit v0.1.0 --draft=false --latest
   ```

Publishing the draft is a separate delivery action. Do not publish it while
the workflow is running or incomplete. GitHub automatically adds source archive
links; these are separate from the tested binary and deployment attachments.

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
one versioned image published without a complete draft: fix the failure and
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
make build VERSION=v0.1.0
./bin/burrow version
make web-install
BURROW_E2E_BINARY="$PWD/bin/burrow" make test-e2e
make compose-config COMPOSE_ENV=.env.example
```

The downloadable Linux binary comes from the Debian bookworm container build
and requires glibc 2.36+. Do not use a locally built macOS binary as the Linux
asset or disable CGO: SQLite depends on it. No multi-platform support is implied.
