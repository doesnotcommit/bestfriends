# Repository Guidelines

## Project Structure & Module Organization

- cmd/app/ — main HTTP server (SSR) with embedded templates
  - cmd/app/templates/ — HTML templates (home.gohtml, add.gohtml)
- cmd/migrate/ — standalone database migrator
- migrations/ — SQL files applied by the migrator (ordered lexicographically)
- Makefile — ko-based container build targets and local run helpers
- .ko.yaml — ko build configuration for app and migrator images
- go.mod, go.sum — module and dependencies

## Build, Test, and Development Commands

```bash
# Run the app locally (requires LEADERBOARD_DB_URL)
make run-local        # equivalent to: go run ./cmd/app

# Run the migrator locally (applies pending migrations)
make migrate-local    # equivalent to: go run ./cmd/migrate

# Build container images with ko (requires KO_DOCKER_REPO)
make build TAG=v0.0.1           # builds app image
make build-migrate TAG=v0.0.1   # builds migrator image

# Plain Go builds (binaries in current dir)
go build -o app ./cmd/app
go build -o migrate ./cmd/migrate
```

## Coding Style & Naming Conventions

- Indentation: tabs (Go standard), gofmt/goimports formatting
- Files/packages: lowercase, short names; templates use .gohtml
- Exported names: PascalCase; unexported: camelCase (Go conventions)
- Linting/formatting: use `go fmt ./...`; consider `go vet ./...` before commits

## Testing Guidelines

- Framework: Go standard `testing` (no tests currently in repo)
- Test files: `*_test.go` colocated with code
- Run tests: `go test ./...`
- Coverage: no explicit requirement

## Commit & Pull Request Guidelines

- Commit messages: concise, imperative. No enforced convention in repo
- PRs: include summary of changes, how to run, and any schema changes
- Database changes: add a new numbered SQL file under `migrations/` and run migrator
- Branch naming: not enforced; prefer `feat/…`, `fix/…`, `chore/…`

---

# Repository Tour

## 🎯 What This Repository Does

bestfriends is a minimal Go SSR app backed by CockroachDB that lets anonymous users upload exhibits (profiles with a photo) and upvote them. Includes a standalone migrator to apply SQL migrations.

**Key responsibilities:**
- Render list/search/pagination and submission form via html/template
- Store photos (JPEG-encoded) and metadata in CockroachDB
- Enforce 60-minute per-profile vote rate limiting using a helper table
- Provide health/readiness endpoints for k8s

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
- Templates (embed.FS): add.gohtml (submission), home.gohtml (listing/search/paging+vote)
- Image pipeline: decode JPEG/PNG, resize (nearest), re-encode JPEG under 500KB
- Rate limiter: votes_recent table checked within serializable transaction
- Migrator: applies SQL files in `migrations/` once, tracked via schema_migrations

### Data Flow
1. GET / — optional `q`, `page`, `page_size` params; query profiles ordered by votes desc, created desc
2. GET /add — render submission form
3. POST /profiles — parse multipart, validate, process image, insert row into profiles
4. POST /profiles/{id}/vote — within tx: check votes_recent 60m window; insert + increment votes_count
5. GET /profiles/{id}/photo — serve photo bytes with ETag and Cache-Control
6. GET /healthz, /readyz — liveness/readiness

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
| cmd/app/main.go | HTTP server, handlers, DB access, templates, image processing | Add endpoints, change query logic, tweak limits |
| cmd/app/templates/*.gohtml | SSR templates | Adjust UI/layout/text |
| cmd/migrate/main.go | Simple migration runner | Extend migration logic or error handling |
| migrations/001_init.sql | Base schema (profiles, indexes) | Evolve schema; add columns/indexes |
| migrations/002_votes_recent.sql | Rate-limit support table + index | Tune rate limiting strategy |
| Makefile | ko build targets and local helpers | Change image tags/platforms or dev workflow |
| .ko.yaml | ko build config (images, flags, labels) | Adjust build options, base image, labels |
| README.md | High-level overview and ops notes | Update docs, env var explanations |

---

## 🔧 Technology Stack

### Core Technologies
- Language: Go 1.22 (from go.mod)
- Web: net/http, html/template (SSR)
- Database: CockroachDB via Postgres driver github.com/lib/pq
- Images: image, image/jpeg (PNG decode supported; encoded as JPEG today)

### Key Libraries
- github.com/lib/pq — Postgres driver
- log/slog — structured logging

### Development Tools
- ko — container builds, multi-arch; configured in .ko.yaml
- Make — thin wrapper for ko and local runs
- go toolchain — build/test/format/vet

---

## 🌐 External Dependencies

- CockroachDB (or any Postgres-compatible DB) via LEADERBOARD_DB_URL
- Container registry for ko builds (KO_DOCKER_REPO)

### Environment Variables

Required at runtime/build where applicable:
- LEADERBOARD_DB_URL — Postgres/Cockroach connection string (required by app and migrator)

Optional:
- LEADERBOARD_ADDR — server address (default :8080)
- LEADERBOARD_PAGE_SIZE_DEFAULT — default page size (default 20, max 100)
- LEADERBOARD_DEBUG_HTTP — set true/1 to log request headers
- LEADERBOARD_MIGRATIONS_DIR — custom path for SQL migrations (default migrations)
- KO_DOCKER_REPO, KO_TAG, KO_GIT_COMMIT, KO_IMAGE_SOURCE — ko build metadata

---

## 🔄 Common Workflows

### Apply schema migrations locally
1. Create/update SQL files in migrations/
2. Export DB URL: `export LEADERBOARD_DB_URL=postgresql://…`
3. Run: `make migrate-local`

### Run the app locally
1. Ensure the database is reachable and migrated
2. `export LEADERBOARD_DB_URL=postgresql://…`
3. `make run-local` (or `go run ./cmd/app`)

### Build and push images with ko
1. `export KO_DOCKER_REPO=ghcr.io/you/bestfriends`
2. `make build TAG=vX.Y.Z` and/or `make build-migrate TAG=vX.Y.Z`

---

## 📈 Performance & Scale

- DB queries are simple index-backed selects; primary sort index defined
- Image processing is CPU-bound and synchronous per request; single node
- ReadHeaderTimeout set to 10s; basic structured request logging

---

## 🚨 Things to Be Careful About

### Security Considerations
- File uploads accepted up to 1MB; decoded and re-encoded server-side
- No authentication; all endpoints are public
- If LEADERBOARD_DEBUG_HTTP is enabled, headers are logged (values >2KB are truncated)

### Data Handling
- Photos stored as BYTES; content type currently image/jpeg though schema default mentions image/webp
- votes_recent has unbounded growth proportional to accepted votes (no TTL job yet)


Updated at: 2025-11-04 UTC
