# Copied from github.com/brienze1/quant (Makefile), MIT License,
# Copyright (c) 2026 Luis Brienze. Modified for Phonon; see NOTICE.md.
export PATH := $(HOME)/.gvm/pkgsets/go1.26.3/global/bin:$(HOME)/.nvm/versions/node/v24.16.0/bin:$(PATH)
.PHONY: dev helper whisper build install release test test-go test-fe check
APP := build/bin/Phonon.app
# VERSION is baked into the binary: make build VERSION=v0.0.1-test. make release sets it to the tag.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
dev: whisper
	wails dev
helper:
	bash helpers/audiocap/build.sh
# whisper.cpp as static libraries for cgo; skipped when already built for the pinned version.
whisper:
	bash scripts/build-whisper.sh
build: helper whisper
	MACOSX_DEPLOYMENT_TARGET=14.0 CGO_CFLAGS=-mmacosx-version-min=14.0 CGO_LDFLAGS=-mmacosx-version-min=14.0 wails build -platform darwin/arm64 -ldflags "-X github.com/GabiHert/phonon/internal/infra.Version=$(VERSION)"
	touch frontend/dist/.gitkeep
	cp helpers/audiocap/build/audiocap $(APP)/Contents/MacOS/audiocap
	cp build/darwin/iconfile.icns $(APP)/Contents/Resources/iconfile.icns
	bash scripts/sign-bundle.sh $(APP)
	bash scripts/check-bundle.sh
	bash scripts/make-notted-bridge.sh
install:
	bash scripts/install.sh
# Tests, builds, signs, tags and publishes a GitHub Release from this Mac: make release [V=vX.Y.Z]
release:
	bash scripts/release.sh $(V)
test-go: whisper
	go vet ./...
	go test ./...
test-fe:
	cd frontend && npm ci && npm run typecheck && npm test
check:
	bash scripts/check-clean.sh
	bash scripts/check-invariants.sh
	bash scripts/test-checks.sh
	bash scripts/test-signing.sh
	bash scripts/test-changelog.sh
	bash scripts/test-install.sh
test: test-go check test-fe
