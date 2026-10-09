#!/usr/bin/env bash
set -euo pipefail

# Must run in the checked-out tag, with GH_TOKEN and GITHUB_REPOSITORY set.
tag=${GITHUB_REF_NAME:?Set GITHUB_REF_NAME}
[[ ${GITHUB_REF_TYPE:-} == tag ]] || { echo 'Release requires a tag ref.' >&2; exit 1; }
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo 'Expected a stable version tag such as v0.1.0.' >&2; exit 1;
}
sha=$(git rev-parse HEAD)
[[ $sha == "${GITHUB_SHA:?Set GITHUB_SHA}" ]] || { echo 'Checkout does not match the workflow commit.' >&2; exit 1; }
[[ $(git rev-parse "refs/tags/$tag^{commit}") == "$sha" ]] || { echo 'Version tag does not match checkout.' >&2; exit 1; }
git fetch --no-tags origin main
git merge-base --is-ancestor "$sha" FETCH_HEAD || { echo 'Release commit must belong to main.' >&2; exit 1; }

# Main CI requires either a matching successful PR verification record or full
# regression, then builds, smoke-tests and publishes both images. Require that
# complete run for this exact commit; PR CI alone cannot authorize a release.
# API failures are fatal. Never accept another commit's successful CI.
runs=$(gh api --method GET "repos/$GITHUB_REPOSITORY/actions/workflows/ci.yml/runs" \
  -f branch=main -f event=push -f head_sha="$sha" -f per_page=100)
jq -e --arg sha "$sha" 'any(.workflow_runs[];
  .head_sha == $sha and .head_branch == "main" and .event == "push" and
  .status == "completed" and .conclusion == "success")' <<<"$runs" >/dev/null || {
  echo 'No successful main push CI for this commit. Wait for CI, then rerun the release.' >&2; exit 1;
}

releases=$(gh api --paginate "repos/$GITHUB_REPOSITORY/releases?per_page=100")
jq -se --arg tag "$tag" '[.[][] | select(.tag_name == $tag)] | all(.draft == true)' <<<"$releases" >/dev/null || {
  echo 'This release is already published; create a new version instead.' >&2; exit 1;
}
test -f "docs/releases/$tag.md"
printf 'version=%s\ncommit=%s\nbuild_time=%s\n' "$tag" "$sha" "$(git show -s --format=%cI HEAD)" >>"${GITHUB_OUTPUT:?Set GITHUB_OUTPUT}"
