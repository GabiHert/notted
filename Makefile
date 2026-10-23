# Copied from github.com/brienze1/quant (Makefile), MIT License,
# Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.
export PATH := $(HOME)/.gvm/pkgsets/go1.26.3/global/bin:$(HOME)/.nvm/versions/node/v24.16.0/bin:$(PATH)
.PHONY: dev build test test-go test-fe check
dev:
	wails dev
build:
	wails build -platform darwin/arm64
	touch frontend/dist/.gitkeep
test-go:
	go vet ./...
	go test ./...
test-fe:
	cd frontend && npm ci && npm run typecheck && npm test
check:
	bash scripts/check-clean.sh
	bash scripts/check-invariants.sh
	bash scripts/test-checks.sh
test: test-go check test-fe
