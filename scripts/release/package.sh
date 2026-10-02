#!/usr/bin/env bash
set -euo pipefail
# macOS tar otherwise adds AppleDouble metadata files to locally checked archives.
export COPYFILE_DISABLE=1

tag=${1:?Usage: package.sh v0.1.0 /path/to/linux-amd64-binary /output/directory}
binary=${2:?Provide the Linux amd64 binary extracted from the release image}
output=${3:?Provide an output directory}
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 1
test -x "$binary"
file -b "$binary" | grep -q 'ELF 64-bit.*x86-64' || {
  echo 'The release asset must be a Linux amd64 ELF binary.' >&2; exit 1;
}
mkdir -p "$output"
output=$(cd "$output" && pwd)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
native="burrow_${tag}_linux_amd64"
deploy="burrow_${tag}_deploy"
mkdir -p "$staging/$native/configs" "$staging/$deploy"
cp "$binary" "$staging/$native/burrow"
cp configs/config.yaml "$staging/$native/configs/"
cp LICENSE "$staging/$native/"
cp docs/releases/INSTALL.md "$staging/$native/README.md"
cp docker-compose.yml .env.example LICENSE "$staging/$deploy/"
cp docs/releases/INSTALL.md "$staging/$deploy/README.md"
# Keep the canonical root Compose byte-for-byte; select the image in .env.
sed "s|^BURROW_IMAGE=.*|BURROW_IMAGE=ghcr.io/arkgravity/burrow:$tag|" .env.example >"$staging/$deploy/.env.example"
cp docs/releases/INSTALL.md "$output/INSTALL.md"
for name in "$native" "$deploy"; do
  tar -czf "$output/$name.tar.gz" -C "$staging" "$name"
done
(cd "$output" && sha256sum "${native}.tar.gz" "${deploy}.tar.gz" INSTALL.md >SHA256SUMS)
