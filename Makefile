# Copied from github.com/brienze1/quant (Makefile), MIT License,
# Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
export PATH := $(HOME)/.gvm/pkgsets/go1.26.3/global/bin:$(HOME)/.nvm/versions/node/v24.16.0/bin:$(PATH)
.PHONY: dev helper build test test-go test-fe check
APP := build/bin/Notted.app
dev:
	wails dev
helper:
	bash helpers/audiocap/build.sh
build: helper
	MACOSX_DEPLOYMENT_TARGET=14.0 CGO_CFLAGS=-mmacosx-version-min=14.0 CGO_LDFLAGS=-mmacosx-version-min=14.0 wails build -platform darwin/arm64
	touch frontend/dist/.gitkeep
	cp helpers/audiocap/build/audiocap $(APP)/Contents/MacOS/audiocap
	bash scripts/fixup-macos-dylibs.sh $(APP)/Contents/MacOS/Notted
	bash scripts/sign-bundle.sh $(APP)
	bash scripts/check-bundle.sh
test-go:
	go vet ./...
	go test ./...
test-fe:
	cd frontend && npm ci && npm run typecheck && npm test
check:
	bash scripts/check-clean.sh
	bash scripts/check-invariants.sh
	bash scripts/test-checks.sh
	bash scripts/test-signing.sh
test: test-go check test-fe
