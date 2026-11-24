#!/usr/bin/env bash
# Purpose: build the Linux setup download (ND-278): a .tar.gz holding the program, the setup
# launcher "Install Network Doctor.desktop" and GUIDE.md, in one folder, exec bits kept.
# Owner: Rafiq. Version 0.1.0 (2026-11-20: first build).
#
# Use: bash scripts/package-linux.sh [amd64|arm64]   (default: amd64)
# Output: dist/release/NetworkDoctor-Setup-<version>-linux-<arch>.tar.gz, and its size printed.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

arch="${1:-amd64}"
case "$arch" in
  amd64 | arm64) ;;
  *) echo "Unknown chip $arch: use amd64 or arm64." >&2; exit 2 ;;
esac

# The version is the one the program itself reports.
version="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' internal/app/app.go)"
if [ -z "$version" ]; then
  echo "The version was not found in internal/app/app.go." >&2
  exit 1
fi

folder="NetworkDoctor-Setup-$version"
stage="dist/stage/linux-$arch"
rm -rf "$stage"
mkdir -p "$stage/$folder" dist/release

echo "Building the Linux program ($arch), version $version"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
  go build -trimpath -ldflags '-s -w' -o "$stage/$folder/netdoctor" ./cmd/netdoctor

# The setup launcher's text comes from the program itself, so it always matches the code.
# A copy built for this machine writes it, since the Linux build may be for another chip.
CGO_ENABLED=0 go build -trimpath -o "$stage/writer" ./cmd/netdoctor
"$stage/writer" install --write-setup-files "$stage/$folder" --system linux
rm -f "$stage/writer" "$stage/writer.exe"

cp docs/GUIDE.md "$stage/$folder/GUIDE.md"
chmod 0755 "$stage/$folder/netdoctor" "$stage/$folder/Install Network Doctor.desktop"
chmod 0644 "$stage/$folder/GUIDE.md"

out="dist/release/NetworkDoctor-Setup-$version-linux-$arch.tar.gz"
rm -f "$out"
# tar keeps each file's mode, so the program and the launcher stay runnable.
tar -C "$stage" --owner=0 --group=0 --numeric-owner -czf "$out" "$folder"

echo "Contents of $out:"
tar -tvzf "$out"
echo "Sizes:"
ls -l "$stage/$folder/netdoctor" "$out" | awk '{print $5, $NF}'
