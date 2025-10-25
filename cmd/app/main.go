package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

//go:embed templates/*
var templatesFS embed.FS

// Configurable constants (can be overridden via env)
const (
	defaultAddr            = ":8080"
	defaultPageSize        = 20
	maxPageSize            = 100
	maxUploadAcceptBytes   = 1 * 1024 * 1024 // 1MB input
	maxStoredImageBytes    = 500 * 1024       // 500KB in DB
	maxImageWidth          = 1024
	defaultTrustedIPHeader = "X-Forwarded-For"
)

type Config struct {
	Addr            string
	DBURL           string
	TrustedIPHeader string
	PageSizeDefault int
	DebugHTTP       bool
}

type Server struct {
	log    *slog.Logger
	tmpl   *template.Template
	db     *sql.DB
	cfg    Config
}

type Profile struct {
	ID              string
	FullName        string
	Country         string
	City            string
	Description     string
	Votes           int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := loadConfig()

	ctx := context.Background()
	if err := run(ctx, logger, cfg); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func loadConfig() Config {
	addr := getenv("LEADERBOARD_ADDR", defaultAddr)
	dburl := getenv("LEADERBOARD_DB_URL", "")
	trusted := getenv("LEADERBOARD_TRUSTED_IP_HEADER", defaultTrustedIPHeader)
	ps := defaultPageSize
	if v := os.Getenv("LEADERBOARD_PAGE_SIZE_DEFAULT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxPageSize {
			ps = n
		}
	}
	debugHTTP := strings.EqualFold(os.Getenv("LEADERBOARD_DEBUG_HTTP"), "1") || strings.EqualFold(os.Getenv("LEADERBOARD_DEBUG_HTTP"), "true")
	return Config{Addr: addr, DBURL: dburl, TrustedIPHeader: trusted, PageSizeDefault: ps, DebugHTTP: debugHTTP}
}

func run(ctx context.Context, logger *slog.Logger, cfg Config) error {
	if cfg.DBURL == "" {
		return fmt.Errorf("DB_URL is required")
	}

	db, err := sql.Open("postgres", cfg.DBURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	if err := migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"inc": func(i int) int { return i + 1 },
		"dec": func(i int) int { if i>1 { return i-1 }; return 1 },
	}).ParseFS(templatesFS, "templates/*.gohtml")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	s := &Server{log: logger, tmpl: tmpl, db: db, cfg: cfg}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/add", s.handleAdd)
	mux.HandleFunc("/profiles", s.handleCreateProfile)
	mux.HandleFunc("/profiles/", s.handleProfileSubroutes) // /profiles/{id}/photo and /profiles/{id}/vote
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.PingContext(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	h := http.Handler(mux)
	if cfg.DebugHTTP { h = debugRequestLogger(logger, h) }
	srv := &http.Server{Addr: cfg.Addr, Handler: logMiddleware(logger, h), ReadHeaderTimeout: 10 * time.Second}
	logger.Info("listening", "addr", cfg.Addr)
	return srv.ListenAndServe()
}

func migrate(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS profiles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			full_name STRING NOT NULL,
			location_country STRING NOT NULL,
			location_city STRING NOT NULL,
			description STRING(160) NOT NULL,
			photo_webp BYTES NOT NULL,
			photo_content_type STRING NOT NULL DEFAULT 'image/webp',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			votes_count INT NOT NULL DEFAULT 0,
			search_text STRING NOT NULL AS (lower(full_name || ' ' || location_country || ' ' || location_city || ' ' || description)) STORED
		);`,
		`CREATE INDEX IF NOT EXISTS idx_profiles_sort ON profiles (votes_count DESC, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_profiles_search ON profiles (search_text);`,
		`CREATE TABLE IF NOT EXISTS votes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			voter_ip INET NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (profile_id, voter_ip)
		);`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := clampAtoi(r.URL.Query().Get("page"), 1, 1<<31-1, 1)
	size := clampAtoi(r.URL.Query().Get("page_size"), 1, maxPageSize, s.cfg.PageSizeDefault)
	offset := (page - 1) * size

	ctx := r.Context()
	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id::string, full_name, location_country, location_city, description, votes_count, created_at, updated_at
			FROM profiles
			ORDER BY votes_count DESC, created_at DESC
			LIMIT $1 OFFSET $2`, size, offset)
	} else {
		like := "%" + strings.ToLower(q) + "%"
		rows, err = s.db.QueryContext(ctx, `
			SELECT id::string, full_name, location_country, location_city, description, votes_count, created_at, updated_at
			FROM profiles
			WHERE search_text LIKE $1
			ORDER BY votes_count DESC, created_at DESC
			LIMIT $2 OFFSET $3`, like, size, offset)
	}
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var list []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.FullName, &p.Country, &p.City, &p.Description, &p.Votes, &p.CreatedAt, &p.UpdatedAt); err != nil {
			http.Error(w, "scan error", http.StatusInternalServerError)
			return
		}
		list = append(list, p)
	}

	data := map[string]any{
		"Profiles": list,
		"Query":    q,
		"Page":     page,
		"PageSize": size,
	}
	if err := s.tmpl.ExecuteTemplate(w, "home.gohtml", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.tmpl.ExecuteTemplate(w, "add.gohtml", nil); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseMultipartForm(maxUploadAcceptBytes); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	fullName := strings.TrimSpace(r.FormValue("full_name"))
	country := strings.TrimSpace(r.FormValue("country"))
	city := strings.TrimSpace(r.FormValue("city"))
	desc := strings.TrimSpace(r.FormValue("description"))
	if fullName == "" || country == "" || city == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	if len(desc) > 160 {
		http.Error(w, "description too long", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "photo required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > maxUploadAcceptBytes {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	// Read uploaded bytes with a cap
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, file, maxUploadAcceptBytes+1); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if buf.Len() > maxUploadAcceptBytes {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	processed, contentType, err := processImageToWebP(buf.Bytes(), maxImageWidth, maxStoredImageBytes)
	if err != nil {
		http.Error(w, "image processing failed", http.StatusBadRequest)
		return
	}

	// Insert profile
	err = withTx(r.Context(), s.db, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(r.Context(), `
			INSERT INTO profiles (full_name, location_country, location_city, description, photo_webp, photo_content_type)
			VALUES ($1,$2,$3,$4,$5,$6)
			RETURNING id::string
		`, fullName, country, city, desc, processed, contentType).Scan(&id)
		if err != nil { return err }
		return nil
	})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleProfileSubroutes(w http.ResponseWriter, r *http.Request) {
	// Expect /profiles/{id}/photo or /profiles/{id}/vote
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/profiles/"), "/")
	if len(parts) < 2 { http.NotFound(w, r); return }
	id, action := parts[0], parts[1]
	switch action {
	case "photo":
		s.servePhoto(w, r, id)
	case "vote":
		if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
		s.voteProfile(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) servePhoto(w http.ResponseWriter, r *http.Request, id string) {
	var b []byte
	var ct string
	var updated time.Time
	err := s.db.QueryRowContext(r.Context(), `SELECT photo_webp, photo_content_type, updated_at FROM profiles WHERE id = $1`, id).Scan(&b, &ct, &updated)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	etag := fmt.Sprintf("\"%s-%d\"", id, updated.Unix())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=2592000") // 30 days
	w.Header().Set("Content-Type", ct)
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) voteProfile(w http.ResponseWriter, r *http.Request, id string) {
	ip := clientIP(r, s.cfg.TrustedIPHeader)
	err := withTx(r.Context(), s.db, func(tx *sql.Tx) error {
		// Try insert vote
		_, err := tx.ExecContext(r.Context(), `INSERT INTO votes (profile_id, voter_ip) VALUES ($1, $2) ON CONFLICT (profile_id, voter_ip) DO NOTHING`, id, ip)
		if err != nil { return err }
		// Check if row exists to know if we should increment
		var exists bool
		err = tx.QueryRowContext(r.Context(), `SELECT true FROM votes WHERE profile_id = $1 AND voter_ip = $2`, id, ip).Scan(&exists)
		if err != nil { return err }
		if exists {
			_, err = tx.ExecContext(r.Context(), `UPDATE profiles SET votes_count = votes_count + 1, updated_at = now() WHERE id = $1`, id)
			if err != nil { return err }
		}
		return nil
	})
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func clientIP(r *http.Request, header string) string {
	if header != "" {
		if v := r.Header.Get(header); v != "" {
			// For X-Forwarded-For, take the first IP
			parts := strings.Split(v, ",")
			ip := strings.TrimSpace(parts[0])
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil { return host }
	return r.RemoteAddr
}

// processImageToWebP attempts to decode JPEG/PNG, resize to max width, and encode as JPEG as a pure-Go fallback
// Note: Without CGO/libwebp, high-quality WebP encoding isn't available in stdlib. We'll use JPEG with quality tuning
// but still set content type properly if/when a pure-Go webp encoder is added.
func processImageToWebP(input []byte, maxWidth int, maxBytes int) ([]byte, string, error) {
	img, format, err := image.Decode(bytes.NewReader(input))
	if err != nil { return nil, "", fmt.Errorf("decode: %w", err) }
	_ = format
	// Simple nearest-neighbor resize to max width
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w > maxWidth {
		newW := maxWidth
		newH := int(float64(h) * float64(newW) / float64(w))
		img = resizeNearest(img, newW, newH)
	}
	// Iterate jpeg quality to fit under maxBytes
	for q := 80; q >= 40; q -= 5 {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: q}); err != nil {
			return nil, "", err
		}
		if out.Len() <= maxBytes {
			return out.Bytes(), "image/jpeg", nil
		}
	}
	// Final attempt lower quality
	var out bytes.Buffer
	_ = jpeg.Encode(&out, img, &jpeg.Options{Quality: 35})
	if out.Len() > maxBytes {
		return nil, "", fmt.Errorf("cannot fit image under %d bytes", maxBytes)
	}
	return out.Bytes(), "image/jpeg", nil
}

// Very simple nearest-neighbor resize
func resizeNearest(src image.Image, newW, newH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			sx := b.Min.X + int(float64(x)*float64(w)/float64(newW))
			sy := b.Min.Y + int(float64(y)*float64(h)/float64(newH))
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func withTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil { return err }
	defer func() {
		if p := recover(); p != nil { _ = tx.Rollback(); panic(p) }
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func logMiddleware(l *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		l.Info("req", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

// debugRequestLogger logs HTTP requests (without body) including headers and basic metadata when enabled.
func debugRequestLogger(l *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := map[string][]string{}
		for k, v := range r.Header {
			// Copy headers; avoid logging very large values
			if len(v) > 0 {
				if len(strings.Join(v, ",")) > 2048 {
					headers[k] = []string{"<truncated>"}
				} else {
					headers[k] = v
				}
			}
		}
		l.Info("http.debug",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"remote", r.RemoteAddr,
			"headers", headers,
		)
		next.ServeHTTP(w, r)
	})
}

func clampAtoi(s string, min, max, def int) int {
	if s == "" { return def }
	n, err := strconv.Atoi(s)
	if err != nil { return def }
	if n < min { return min }
	if n > max { return max }
	return n
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" { return v }
	return def
}
