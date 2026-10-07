#!/usr/bin/env bash
set -euo pipefail

# Only the post-verification publishing jobs call this script with registry credentials.
tag=${1:?Usage: publish-images.sh tag ci|release /image/directory /output/IMAGES.txt}
kind=${2:?Provide ci or release}
input=${3:?Provide saved image-amd64.tar and image-arm64.tar}
output=${4:?Provide the image digest output path}
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ || $tag =~ ^main-[0-9a-f]{7}$ ]] || exit 1
[[ $kind == ci || $kind == release ]] || exit 1
commit=${GITHUB_SHA:?Set the workflow commit}
repository=$(printf '%s' "${GITHUB_REPOSITORY:?Set the GitHub repository}" | tr '[:upper:]' '[:lower:]')
registries=("ghcr.io/$repository" "docker.io/${DOCKERHUB_NAMESPACE:?Set the Docker Hub namespace}/burrow")
architectures=(amd64 arm64)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
mkdir -p "$staging/anonymous"

# Require both artifacts and validate both local images before any push.
for arch in "${architectures[@]}"; do test -f "$input/image-$arch.tar"; done
config_ids=()
platform_digests=()
for arch in "${architectures[@]}"; do
  local_image="burrow:$kind-$arch"
  docker load --input "$input/image-$arch.tar"
  config_id=$(docker image inspect "$local_image" --format '{{json .}}' | \
    jq -er --arg arch "$arch" --arg commit "$commit" '
      select(.Os == "linux" and .Architecture == $arch and
        .Config.Labels["org.opencontainers.image.revision"] == $commit) |
      .Id | select(test("^sha256:[0-9a-f]{64}$"))')
  config_ids+=("$config_id")
  # Compute the manifest identity from the tested archive before registry writes.
  digest=$(crane digest --tarball "$input/image-$arch.tar")
  [[ $digest =~ ^sha256:[0-9a-f]{64}$ ]] || exit 1
  platform_digests+=("$digest")
done

index_digest=''
for repository in "${registries[@]}"; do
  sources=()
  for i in 0 1; do
    arch=${architectures[$i]}
    image="$repository@${platform_digests[$i]}"
    # Upload the saved image by digest; only the final index receives a tag.
    crane push "$input/image-$arch.tar" "$image"
    # The pushed manifest must reference the exact saved image configuration.
    docker buildx imagetools inspect "$image" --raw | \
      jq -e --arg config "${config_ids[$i]}" '.config.digest == $config' >/dev/null
    digest=$(docker buildx imagetools inspect "$image" --format '{{json .Manifest}}' | \
      jq -er '.digest | select(test("^sha256:[0-9a-f]{64}$"))')
    [[ $digest == "${platform_digests[$i]}" ]] || { echo "Registry digest mismatch for $arch" >&2; exit 1; }
    sources+=("$image")
    printf '%s %s\n' "$image" "$digest" >>"$staging/platform-images.txt"
  done

  image="$repository:$tag"
  docker buildx imagetools create --tag "$image" "${sources[@]}"
  # Anonymous access must work and the index must contain exactly both tested images.
  DOCKER_CONFIG="$staging/anonymous" docker buildx imagetools inspect "$image" --raw | \
    jq -e --arg amd64 "${platform_digests[0]}" --arg arm64 "${platform_digests[1]}" '
      .schemaVersion == 2 and
      ([.manifests[] | {os: .platform.os, arch: .platform.architecture, digest}] | sort_by(.arch)) ==
      [{os: "linux", arch: "amd64", digest: $amd64}, {os: "linux", arch: "arm64", digest: $arm64}]' >/dev/null
  digest=$(DOCKER_CONFIG="$staging/anonymous" docker buildx imagetools inspect "$image" --format '{{json .Manifest}}' | \
    jq -er '.digest | select(test("^sha256:[0-9a-f]{64}$"))')
  if [[ -z $index_digest ]]; then
    index_digest=$digest
  else
    [[ $digest == "$index_digest" ]] || { echo 'Registry index digest mismatch' >&2; exit 1; }
  fi
  printf '%s %s\n' "$image" "$digest" >>"$staging/index-images.txt"
done

mkdir -p "$(dirname "$output")"
cat "$staging/index-images.txt" "$staging/platform-images.txt" >"$output"
