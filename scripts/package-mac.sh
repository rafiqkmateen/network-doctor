#!/usr/bin/env bash
# Purpose: build the Mac setup download (ND-276, ND-277): a zip holding "Network Doctor Setup.app"
# (one program for both Apple and Intel chips, made with lipo, the guide in its Resources folder,
# signed ad hoc) and "How to open Network Doctor Setup.html".
# Owner: Rafiq. Version 0.1.0 (2026-11-20: first build).
#
# Use: bash scripts/package-mac.sh   (on a Mac: lipo, codesign and ditto are Mac tools)
# Output: dist/release/NetworkDoctor-Setup-<version>-mac.zip, and the sizes printed.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

version="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' internal/app/app.go)"
if [ -z "$version" ]; then
  echo "The version was not found in internal/app/app.go." >&2
  exit 1
fi

stage="dist/stage/mac"
rm -rf "$stage"
mkdir -p "$stage/zip" dist/release

for arch in arm64 amd64; do
  echo "Building the Mac program ($arch), version $version"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" \
    go build -trimpath -ldflags '-s -w' -o "$stage/netdoctor-$arch" ./cmd/netdoctor
done

# The app's Info.plist, its start file and the open page come from the program itself.
"$stage/netdoctor-$(go env GOARCH)" install --write-setup-files "$stage/zip" --system mac

app="$stage/zip/Network Doctor Setup.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/netdoctor" "$stage/netdoctor-arm64" "$stage/netdoctor-amd64"
chmod 0755 "$app/Contents/MacOS/netdoctor" "$app/Contents/MacOS/Network Doctor Setup"
cp docs/GUIDE.md "$app/Contents/Resources/GUIDE.md"
echo "Chips in the program: $(lipo -archs "$app/Contents/MacOS/netdoctor")"

# Ad-hoc signing (no certificate): the program first, then the whole app, after every file is in
# place, since a later change would break the seal.
codesign --force --sign - "$app/Contents/MacOS/netdoctor"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"

out="dist/release/NetworkDoctor-Setup-$version-mac.zip"
rm -f "$out"
# ditto keeps the modes and the signature's extended attributes, as Finder's own zip does. With no
# --keepParent it puts the folder's two items at the top of the zip.
ditto -c -k --sequesterRsrc "$stage/zip" "$out"

echo "Contents of $out:"
unzip -l "$out"
echo "Sizes:"
ls -l "$app/Contents/MacOS/netdoctor" "$out" | awk '{print $5, $NF}'
