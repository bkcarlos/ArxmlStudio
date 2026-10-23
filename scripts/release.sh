#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
release_version=${1:-v0.3.0}
if ! printf '%s\n' "$release_version" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  printf 'Expected a version such as v0.3.0\n' >&2
  exit 1
fi
test "$(node -p "require('./frontend/package.json').version")" = "${release_version#v}" || { printf 'Version does not match frontend/package.json\n' >&2; exit 1; }
npm --prefix frontend ci
npm --prefix frontend run build
go test -race ./...
go test -tags headless -race ./...
go vet ./...
go vet -tags headless ./...
release_dir="dist/releases/$release_version"
mkdir -p "$release_dir"
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  target_os=${target%/*}
  target_arch=${target#*/}
  for variant in studio server; do
    build_tags=""
    if [ "$variant" = server ]; then build_tags=headless; fi
    package_name="arxml-${variant}_${release_version}_${target_os}_${target_arch}"
    package_dir="$release_dir/$package_name"
    mkdir -p "$package_dir/examples" "$package_dir/docs"
    binary_name="arxml-$variant"
    if [ "$target_os" = windows ]; then binary_name="$binary_name.exe"; fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -tags "$build_tags" -trimpath \
      -ldflags="-s -w -X main.version=${release_version#v}" -o "$package_dir/$binary_name" .
    cp README.md THIRD_PARTY_NOTICES.txt "$package_dir/"
    cp docs/backend.md "$package_dir/docs/"
    cp examples/*.arxml examples/export-settings.json "$package_dir/examples/"
    if [ "$target_os" = windows ]; then
      (cd "$release_dir" && zip -qr "$package_name.zip" "$package_name")
    else
      COPYFILE_DISABLE=1 tar -czf "$release_dir/$package_name.tar.gz" -C "$release_dir" "$package_name"
    fi
  done
done
(cd "$release_dir" && shasum -a 256 ./*.tar.gz ./*.zip > SHA256SUMS.txt)
printf 'Release assets: %s\n' "$release_dir"
