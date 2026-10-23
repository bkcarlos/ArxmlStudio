#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
CGO_ENABLED=0 go build -tags headless -trimpath -ldflags="-s -w" -o dist/arxml-server .
