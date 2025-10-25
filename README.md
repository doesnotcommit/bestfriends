Hall of Shame (bestfriends)

Minimal SSR Go app using CockroachDB. Anonymous profile uploads and upvotes.

Decisions
- SSR templates (html/template)
- Single substring search across name, country, city, description
- Accept images up to 1MB; store <= 500KB after resize; max width 1024px
- No CGO: currently stores JPEG to meet <=500KB; content-type image/jpeg
- Photo caching via ETag and Cache-Control
- One upvote per IP per profile; configurable trusted header

Env
- LEADERBOARD_DB_URL: CockroachDB connection string (postgres-compatible)
- LEADERBOARD_ADDR: server address, default :8080
- LEADERBOARD_TRUSTED_IP_HEADER: default X-Forwarded-For
- LEADERBOARD_PAGE_SIZE_DEFAULT: default 20

Run
- go build ./cmd/app && ./app

Endpoints
- GET /           list + search + pagination
- GET /add        new profile form
- POST /profiles  create profile (multipart)
- POST /profiles/{id}/vote upvote
- GET /profiles/{id}/photo image
- GET /healthz, /readyz

Schema
- Auto-migrated at startup. See migrate() in cmd/app/main.go

Notes
- If we later allow pure-Go WebP encoding with acceptable quality/size, we can switch content-type to image/webp and keep the same table column.
