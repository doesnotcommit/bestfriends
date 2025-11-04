# Lean Makefile using ko and .ko.yaml defaults

# Usage:
#   export KO_DOCKER_REPO=ghcr.io/you/bestfriends
#   make build TAG=v0.0.5
#   make build-migrate TAG=v0.0.5

# Parameters (overridable)
TAG        ?= v0.0.1
PLATFORMS  ?=           # override to e.g. linux/arm64,linux/amd64; defaults from .ko.yaml if empty
KO_FLAGS   ?=           # extra ko flags if needed

# Derived/env: You must export KO_DOCKER_REPO in your shell
# KO_TAG/KO_GIT_COMMIT/KO_IMAGE_SOURCE are optional and used for labels in .ko.yaml

.PHONY: help
help:
	@echo "Targets:"
	@echo "  build           - ko build app image (TAG=$(TAG))"
	@echo "  build-migrate   - ko build migrator image (TAG=$(TAG))"
	@echo "  run-local       - go run ./cmd/app (requires LEADERBOARD_DB_URL)"
	@echo "  migrate-local   - go run ./cmd/migrate (requires LEADERBOARD_DB_URL)"
	@echo "Env: export KO_DOCKER_REPO=registry/repo; optional TAG, PLATFORMS, KO_TAG, KO_GIT_COMMIT, KO_IMAGE_SOURCE"

.PHONY: _require-repo
_require-repo:
	@if [ -z "$(KO_DOCKER_REPO)" ]; then \
		echo "KO_DOCKER_REPO must be set (e.g., export KO_DOCKER_REPO=ghcr.io/doesnotcommit/bestfriends)"; \
		exit 1; \
	fi

.PHONY: build
build: _require-repo
	KO_TAG=$(TAG) ko build ./cmd/app \
	  $(if $(PLATFORMS),--platform=$(PLATFORMS),) \
	  --tags=$(TAG) \
	  $(KO_FLAGS)

.PHONY: build-migrate
build-migrate: _require-repo
	KO_TAG=$(TAG) ko build ./cmd/migrate \
	  $(if $(PLATFORMS),--platform=$(PLATFORMS),) \
	  --tags=$(TAG) \
	  $(KO_FLAGS)

.PHONY: run-local
run-local:
	go run ./cmd/app

.PHONY: migrate-local
migrate-local:
	go run ./cmd/migrate
