#!/usr/bin/env bash
set -euo pipefail
export COPYFILE_DISABLE=1

tag=${1:?Usage: assemble.sh vX.Y.Z /input/directory /output/directory}
input=${2:?Provide the downloaded architecture archives and checksum files}
output=${3:?Provide an output directory}
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 1
input=$(cd "$input" && pwd)
# Verify exactly the expected file per architecture before producing attachments.
for arch in amd64 arm64; do
  archive="burrow_${tag}_linux_${arch}.tar.gz"
  expected=$(cat "$input/SHA256SUMS-$arch")
  actual=$(cd "$input" && sha256sum "$archive")
  [[ $actual == "$expected" ]] || { echo "Invalid checksum for $archive" >&2; exit 1; }
done
mkdir -p "$output"
output=$(cd "$output" && pwd)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
deploy="burrow_${tag}_deploy"
mkdir -p "$staging/$deploy"
cp docker-compose.yml .env.example LICENSE "$staging/$deploy/"
sed "s/@VERSION@/$tag/g" docs/releases/INSTALL.md >"$output/INSTALL.md"
cp "$output/INSTALL.md" "$staging/$deploy/README.md"
# Keep the canonical root Compose byte-for-byte; select the multi-platform tag in .env.
sed "s|^BURROW_IMAGE=.*|BURROW_IMAGE=ghcr.io/arkgravity/burrow:$tag|" .env.example >"$staging/$deploy/.env.example"
for arch in amd64 arm64; do
  cp "$input/burrow_${tag}_linux_${arch}.tar.gz" "$output/"
done
tar -czf "$output/$deploy.tar.gz" -C "$staging" "$deploy"
(cd "$output" && sha256sum "burrow_${tag}_linux_amd64.tar.gz" \
  "burrow_${tag}_linux_arm64.tar.gz" "$deploy.tar.gz" INSTALL.md >SHA256SUMS)
