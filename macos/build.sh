#!/bin/sh
# Builds the worker and the app. Run from this directory.
set -e
cd "$(dirname "$0")"
go build -o wake-worker ../cmd/worker
swift build
# A SwiftPM executable is not an app bundle, so the worker goes beside it.
cp wake-worker .build/debug/wake-worker
echo "run it with:  ./.build/debug/wake"
