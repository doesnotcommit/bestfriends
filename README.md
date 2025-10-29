Hall of Shame (bestfriends)

Minimal SSR Go app using CockroachDB. Anonymous profile uploads and upvotes.

Highlights
- Minimal server-side templates (html/template)
- Simple, subtle “gallery” design (no page title), framed photos, plaque-like descriptions, + voting button
- Search: single substring across name, country, city, description
- Images: accept up to 1MB; resize to max width 1024px; store as JPEG <= 500KB (no CGO)
- Photo caching via ETag and Cache-Control (30 days)
- Votes: per-profile 60-minute rolling limit (no IP tracking). Sort by votes desc, then created desc
- Built for k8s with a small Docker image (multi-stage build)

Environment variables
- LEADERBOARD_DB_URL: CockroachDB connection string (postgres-compatible). Required
- LEADERBOARD_ADDR: server address, default :8080
- LEADERBOARD_PAGE_SIZE_DEFAULT: default 20 (max 100)
- LEADERBOARD_DEBUG_HTTP: set true/1 to log HTTP requests (headers only; no body)

Build & Run
- Local: go build ./cmd/app && ./app
- Docker: docker build -t bestfriends:latest .
  - docker run -p 8080:8080 -e LEADERBOARD_DB_URL='postgresql://...' bestfriends:latest

Endpoints
- GET /                      list + search + pagination
- GET /add                   new profile form
- POST /profiles             create profile (multipart: full_name, country, city, description, photo)
- POST /profiles/{id}/vote   upvote (subject to 60-minute per-profile limit)
- GET /profiles/{id}/photo   image (cached)
- GET /healthz, /readyz

Schema (managed via external migrations)
Migrations
- Use the standalone migrator:
  - Build: go build -o migrate ./cmd/migrate
  - Run:   LEADERBOARD_DB_URL='postgresql://...' ./migrate
  - Directory: migrations/ (override with LEADERBOARD_MIGRATIONS_DIR)

Schema
- profiles
  - id UUID PRIMARY KEY DEFAULT gen_random_uuid()
  - full_name STRING NOT NULL
  - location_country STRING NOT NULL
  - location_city STRING NOT NULL
  - description STRING(160) NOT NULL
  - photo_webp BYTES NOT NULL           // currently JPEG payload
  - photo_content_type STRING NOT NULL  // currently image/jpeg
  - created_at, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
  - votes_count INT NOT NULL DEFAULT 0
  - search_text STRING STORED (lower(full_name || ' ' || location_country || ' ' || location_city || ' ' || description))
  - indexes: idx_profiles_sort (votes_count DESC, created_at DESC), idx_profiles_search (search_text)
- votes_recent
  - id UUID PRIMARY KEY DEFAULT gen_random_uuid()
  - profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE
  - created_at TIMESTAMPTZ NOT NULL DEFAULT now()
  - index: idx_votes_recent_profile_created (profile_id, created_at DESC)

Rate limiting behavior
- One successful vote per profile per rolling 60 minutes
- If a vote occurs within the window, the server returns 429 Too Many Requests
- Typed error used internally (ErrorRateLimited) with marker method RateLimited(), asserted via errors.As

Notes
- No thumbnails and no CGO. If we adopt a pure-Go WebP encoder later, we can change content-type to image/webp without schema change
- No explicit housekeeping for votes_recent yet; table growth equals number of accepted votes
