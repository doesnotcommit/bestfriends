# Repository Guidelines

## Project Structure & Module Organization

- cmd/app/ — main HTTP server (SSR) with embedded templates
  - cmd/app/templates/ — HTML templates (home.gohtml, add.gohtml)
- cmd/migrate/ — standalone database migrator
- migrations/ — SQL files applied by the migrator (ordered lexicographically)
- Makefile — ko-based container build targets and local run helpers
- .ko.yaml — ko build configuration for app and migrator images
- go.mod, go.sum — module and dependencies
- README.md — overview, endpoints, schema and run instructions

## Build, Test, and Development Commands

```bash
# Run the app locally (requires LEADERBOARD_DB_URL)
make run-local          # go run ./cmd/app

# Run the migrator locally (applies pending migrations)
make migrate-local      # go run ./cmd/migrate

# Build container images with ko (requires KO_DOCKER_REPO)
make build TAG=v0.0.1           # builds app image
make build-migrate TAG=v0.0.1   # builds migrator image

# Plain Go builds (binaries in current dir)
go build -o app ./cmd/app
go build -o migrate ./cmd/migrate

# Run from built image (example)
docker build -t bestfriends:latest .
docker run -p 8080:8080 -e LEADERBOARD_DB_URL='postgresql://...' bestfriends:latest
```

## Coding Style & Naming Conventions

- Indentation: tabs (Go standard). Format with gofmt/goimports
- Files/packages: lowercase, short names; templates use .gohtml
- Exported identifiers: PascalCase; unexported: camelCase (Go conventions)
- Linting/formatting: run `go fmt ./...`; optional `go vet ./...` before commits

## Testing Guidelines

- Framework: Go standard `testing` (no tests present as of now)
- Test files: `*_test.go` colocated with code
- Running tests: `go test ./...`
- Coverage: no explicit requirement

## Commit & Pull Request Guidelines

- Commit messages: concise, imperative (no enforced convention found)
- PRs: describe changes, how to run locally, and any schema/migration impacts
- Database changes: add a new numbered .sql under `migrations/` and run migrator
- Branch naming: not enforced; suggested: `feat/...`, `fix/...`, `chore/...`

---

# Repository Tour

## 🎯 What This Repository Does

bestfriends is a minimal Go SSR web app backed by CockroachDB (Postgres driver) that lets anonymous users upload exhibits (profiles with a photo) and upvote them. A standalone migrator applies SQL migrations.

**Key responsibilities:**
- Render listing, search, pagination, and submission UI via html/template
- Accept, resize, and store images with metadata in the database
- Enforce 60-minute per-profile vote rate limiting
- Provide health/readiness endpoints for ops

---

## 🏗️ Architecture Overview

### System Context
```
[Browser] → [Go HTTP server (cmd/app)] → [CockroachDB (Postgres wire)]
                                 ↘
                             [Templates]

[Operator] → [Migrator (cmd/migrate)] → [CockroachDB]
```

### Key Components
- cmd/app: HTTP server using net/http, database/sql (driver github.com/lib/pq), html/template
- Templates (embed.FS): add.gohtml (submission), home.gohtml (listing/search/paging + vote)
- Image pipeline: decode JPEG/PNG, resize (nearest), re-encode as JPEG under 500KB (pure Go)
- Rate limiter: votes_recent table checked within serializable transaction
- Migrator: applies SQL files in `migrations/` once, tracked via schema_migrations

### Data Flow
1. GET / — optional `q` filter; fetch profiles ordered by votes desc, created desc (limit 500)
2. GET /add — render submission form
3. POST /profiles — parse multipart, validate, process image, insert into profiles
4. POST /profiles/{id}/vote — in tx: check 60m window in votes_recent; insert + increment votes_count
5. GET /profiles/{id}/photo — return photo bytes with ETag and Cache-Control (30d)
6. GET /healthz, /readyz — liveness/readiness (readyz pings DB)

---

## 📁 Project Structure [Partial Directory Tree]

```
bestfriends/
├── cmd/
│   ├── app/
│   │   ├── main.go
│   │   └── templates/
│   │       ├── add.gohtml
│   │       └── home.gohtml
│   └── migrate/
│       └── main.go
├── migrations/
│   ├── 001_init.sql
│   └── 002_votes_recent.sql
├── Makefile
├── .ko.yaml
├── README.md
├── go.mod
└── go.sum
```

### Key Files to Know

| File | Purpose | When You'd Touch It |
|------|---------|---------------------|
| cmd/app/main.go | HTTP server, handlers, DB access, templates, image processing | Add endpoints, change queries, adjust limits |
| cmd/app/templates/*.gohtml | SSR templates | Modify UI/layout/text |
| cmd/migrate/main.go | Migration runner | Extend migration behavior or logging |
| migrations/001_init.sql | Base schema (profiles, indexes) | Evolve schema; add columns/indexes |
| migrations/002_votes_recent.sql | Rate-limit support table + index | Tune rate limiting strategy |
| Makefile | ko build targets and local helpers | Change image tags/platforms or dev workflow |
| .ko.yaml | ko build configuration | Adjust build options, labels, base image |
| README.md | Overview, endpoints, schema, envs | Update docs as features evolve |

---

## 🔧 Technology Stack

### Core Technologies
- Language: Go 1.22 (go.mod)
- Web/SSR: net/http, html/template
- Database: CockroachDB/Postgres via github.com/lib/pq
- Imaging: image, image/jpeg (PNG decode supported); encoded as JPEG currently

### Key Libraries
- github.com/lib/pq — Postgres driver
- log/slog — structured logging

### Development Tools
- ko — container builds per .ko.yaml
- Make — wrappers for ko and local runs
- Go toolchain — build/test/format/vet

---

## 🌐 External Dependencies

- CockroachDB or Postgres-compatible DB — via LEADERBOARD_DB_URL
- Container registry for ko builds — KO_DOCKER_REPO

### Environment Variables

```bash
# Required
LEADERBOARD_DB_URL=        # Postgres/Cockroach connection string

# Optional
LEADERBOARD_ADDR=:8080
LEADERBOARD_MIGRATIONS_DIR=migrations
LEADERBOARD_DEBUG_HTTP=0   # true/1 enables request header logging
# (README mentions LEADERBOARD_PAGE_SIZE_DEFAULT; not referenced in code as of last update)
```

---

## 🔄 Common Workflows

### Apply schema migrations locally
1. Create/update SQL in `migrations/`
2. `export LEADERBOARD_DB_URL=postgresql://...`
3. `make migrate-local`

### Run the app locally
1. Ensure DB is reachable and migrated
2. `export LEADERBOARD_DB_URL=postgresql://...`
3. `make run-local` or `go run ./cmd/app`

### Build and push images with ko
1. `export KO_DOCKER_REPO=ghcr.io/you/bestfriends`
2. `make build TAG=vX.Y.Z` and/or `make build-migrate TAG=vX.Y.Z`

---

## 📈 Performance & Scale

- Index-backed queries on profiles; sort index ensures predictable performance
- Image processing is CPU-bound and synchronous; consider background jobs for heavy traffic
- HTTP server uses ReadHeaderTimeout=10s; basic structured request logging

---

## 🚨 Things to Be Careful About

### Security Considerations
- Public endpoints; no authentication
- File uploads: max 1MB accepted; server re-encodes to fit under 500KB
- Debug logging may include headers; values >2KB are truncated

### Data Handling
- Schema default sets photo_content_type to image/webp, but server currently stores JPEG; both handled via stored content type
- votes_recent growth equals accepted votes; no TTL/cleanup yet


Updated at: 2025-11-04 UTC
