#!/usr/bin/env bash
# Purpose: fetch Ookla's speed test files for Mac and Linux from install.speedtest.net and print
# each file's name, address and SHA-256 in one block, for the fixed values in
# internal/install/ookla.go (ND-302). It prints nothing else and keeps nothing.
# Owner: Rafiq. Version 0.1.0 (2026-11-20: first build).
#
# Use: bash scripts/parity/ookla-fingerprints.sh
set -euo pipefail

base="https://install.speedtest.net/app/cli"
version="1.2.0"
files=(
  "ookla-speedtest-$version-linux-x86_64.tgz"
  "ookla-speedtest-$version-linux-aarch64.tgz"
  "ookla-speedtest-$version-linux-armhf.tgz"
  "ookla-speedtest-$version-macosx-universal.tgz"
)

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
block=""
missing=0
for f in "${files[@]}"; do
  if curl -fsSL --proto '=https' --tlsv1.2 -o "$work/$f" "$base/$f"; then
    sum="$(sha256sum "$work/$f" | awk '{print $1}')"
    block+="$f"$'\n'"  url:    $base/$f"$'\n'"  sha256: $sum"$'\n'
  else
    block+="$f"$'\n'"  url:    $base/$f"$'\n'"  NOT FOUND: see the file names on https://www.speedtest.net/apps/cli"$'\n'
    missing=$((missing + 1))
  fi
done

echo "===== Ookla $version file fingerprints ====="
printf '%s' "$block"
echo "===== end ====="
[ "$missing" -eq 0 ]
