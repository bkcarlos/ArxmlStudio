#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
npm --prefix frontend ci
npm --prefix frontend run build
mkdir -p dist
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/arxml-studio .
printf 'Built: dist/arxml-studio\n'
