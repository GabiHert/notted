#!/usr/bin/env bash
# Compiles the pure Swift parts with a plain test runner, runs it, then checks
# the WAV files it wrote with wavcheck.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p build
swiftc -swift-version 5 -target arm64-apple-macos13.0 \
  Sources/Events.swift Sources/WavWriter.swift Sources/TrackWriter.swift Tests/main.swift \
  -o build/audiocap-tests
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
build/audiocap-tests "$tmp"
go run ../../scripts/wavcheck -expect-chunk 2 "$tmp"/rotate/*.wav
