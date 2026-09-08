# ABOUTME: Exposes build, validation, installation, and synchronization targets.
# ABOUTME: Runtime dependencies and scanners are provisioned separately.
# Empty PREFIX selects the user-local checkout symlink; an explicit prefix copies.
COMMIT_MESSAGE ?= chore: sync
export PREFIX DESTDIR VERSION RELEASE_BASE_URL COMMIT_MESSAGE

.PHONY: build lint test install sync vulncheck release
build:
	go run ./internal/buildtool build
install:
	go run ./internal/buildtool install
release:
	go run ./internal/buildtool release
sync:
	go run ./internal/buildtool sync
lint:
	go run ./internal/buildtool lint
test:
	go test -race ./...
vulncheck:
	govulncheck ./...
