#!/bin/sh
# Offline alternative to the multi-stage Dockerfile: requires local Go and Docker.
set -eu
cd "$(dirname "$0")/.."
target_arch=${1:-$(go env GOARCH)}
case "$target_arch" in amd64|arm64) ;; *) printf 'Expected amd64 or arm64\n' >&2; exit 1;; esac
mkdir -p dist/container
CGO_ENABLED=0 GOOS=linux GOARCH="$target_arch" go build -tags headless -trimpath -ldflags="-s -w" -o dist/container/arxml-server .
docker build --platform "linux/$target_arch" -f deploy/Dockerfile.binary -t arxml-studio-api:local dist/container
