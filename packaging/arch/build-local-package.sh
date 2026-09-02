#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
out=${1:-"$root/.local-package"}
version=0.1.0
archive="$out/zer0-waypass-$version.tar.gz"
mkdir -p "$out"
tar -czf "$archive" -C "$root" \
  --exclude='.local-package' \
  --exclude='*.test' \
  --transform "s,^,zer0-waypass-$version/," \
  AGENTS.md README.md LICENSE go.mod cmd internal docs noctalia-plugin examples packaging/arch/PKGBUILD
cp "$root/packaging/arch/PKGBUILD" "$out/PKGBUILD"
cd "$out"
makepkg --cleanbuild --clean -f -p PKGBUILD
