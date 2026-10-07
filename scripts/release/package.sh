#!/usr/bin/env bash
set -euo pipefail
# macOS tar otherwise adds AppleDouble metadata files to locally checked archives.
export COPYFILE_DISABLE=1

tag=${1:?Usage: package.sh vX.Y.Z amd64|arm64 /path/to/binary /output/directory}
arch=${2:?Provide amd64 or arm64}
binary=${3:?Provide the binary extracted from the corresponding release image}
output=${4:?Provide an output directory}
[[ $tag =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || exit 1
case "$arch" in
  amd64) machine='x86-64' ;;
  arm64) machine='ARM aarch64' ;;
  *) echo 'Supported release architectures: amd64, arm64.' >&2; exit 1 ;;
esac
test -x "$binary"
file -b "$binary" | grep -q "ELF 64-bit LSB.*$machine" || {
  echo "The release asset must be a Linux $arch ELF binary." >&2; exit 1;
}
mkdir -p "$output"
output=$(cd "$output" && pwd)
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
native="burrow_${tag}_linux_${arch}"
mkdir -p "$staging/$native/configs"
cp "$binary" "$staging/$native/burrow"
cp configs/config.yaml "$staging/$native/configs/"
cp LICENSE "$staging/$native/"
sed "s/@VERSION@/$tag/g" docs/releases/INSTALL.md >"$staging/$native/README.md"
tar -czf "$output/$native.tar.gz" -C "$staging" "$native"
(cd "$output" && sha256sum "$native.tar.gz" >"SHA256SUMS-$arch")
