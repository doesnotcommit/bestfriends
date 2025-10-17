# Repository Guidelines

## Project Structure & Module Organization

Source code is organized in the `cmd/` directory with two main applications:
- `cmd/bycursor/` - Web application for item catalog with image uploads
- `cmd/service/` - Simple HTTP service for testing/demo purposes
- `uploads/` and `data/` directories are created at runtime for file storage

## Build, Test, and Development Commands

```bash
# Build the bycursor web application
go build -o bycursor ./cmd/bycursor

# Build the service application  
go build -o service ./cmd/service

# Run the web application (starts on :8080)
go run ./cmd/bycursor

# Run the service application (starts on :8080)
go run ./cmd/service

# Run tests (if any exist)
go test ./...
```

## Coding Style & Naming Conventions

- **Indentation**: Tabs (Go standard)
- **File naming**: lowercase with underscores for multi-word files
- **Function/variable naming**: camelCase for private, PascalCase for public (Go conventions)
- **Package naming**: lowercase, single word when possible
- **Linting**: Use `go fmt` and `go vet` for standard Go formatting

## Testing Guidelines

- **Framework**: Go standard testing package (`testing`)
- **Test files**: `*_test.go` files alongside source code
- **Running tests**: `go test ./...`
- **Coverage**: No specific requirements defined

## Commit & Pull Request Guidelines

- **Commit format**: `feat: description` (based on recent commits)
- **Examples from repo**: 
  - `feat: look ma, cursor has generated something`
  - `feat: first poc with some customization`
- **PR process**: Standard GitHub workflow (no specific requirements found)
- **Branch naming**: No specific convention defined

---

# Repository Tour

## 🎯 What This Repository Does

**bestfriends** is a Go-based web application for creating and browsing an item catalog with image uploads, plus a simple HTTP service for testing purposes.

**Key responsibilities:**
- Web interface for uploading items with names, descriptions, and images
- File-based storage system using JSON and SHA256 hashing
- Static file serving for uploaded images
- Simple HTTP service for basic request handling

---

## 🏗️ Architecture Overview

### System Context
```
[Web Browser] → [bycursor Web App :8080] → [File System Storage]
                        ↓
                   [uploads/ + data/ directories]

[HTTP Client] → [service App :8080] → [Environment Variables]
```

### Key Components
- **Web Server** - HTTP server handling upload forms and browsing interface
- **File Storage** - JSON files for metadata, uploads directory for images
- **Template Engine** - Embedded HTML templates for web interface
- **Hash Generator** - SHA256 hashing for unique file naming

### Data Flow
1. User accesses web interface at `/` (browse) or `/add` (upload form)
2. Form submissions are processed at `/new` endpoint with file validation
3. Images are saved to `uploads/` directory with SHA256-hashed filenames
4. Item metadata is stored as JSON files in `data/` directory
5. Browse page loads all JSON files and displays items with images

---

## 📁 Project Structure

```
bestfriends/
├── cmd/                    # Application entry points
│   ├── bycursor/          # Web application for item catalog
│   │   └── main.go        # Main web server with upload/browse functionality
│   └── service/           # Simple HTTP service
│       └── main.go        # Basic HTTP handler for testing
├── .git/                  # Git repository metadata
├── go.mod                 # Go module definition
├── LICENSE                # Apache 2.0 license
└── .gitignore            # Git ignore rules for Go projects
```

### Key Files to Know

| File | Purpose | When You'd Touch It |
|------|---------|---------------------|
| `cmd/bycursor/main.go` | Main web application with upload/browse features | Adding new endpoints or modifying UI |
| `cmd/service/main.go` | Simple HTTP service for testing | Creating basic HTTP handlers |
| `go.mod` | Go module dependencies | Adding new Go dependencies |
| `.gitignore` | Git ignore patterns | Excluding new file types from version control |

---

## 🔧 Technology Stack

### Core Technologies
- **Language:** Go 1.24.1 - Modern Go version with latest features
- **Framework:** Standard library `net/http` - No external web framework dependencies
- **Storage:** File system (JSON + static files) - Simple persistence without database
- **Web Server:** Go HTTP server - Built-in HTTP server on port 8080

### Key Libraries
- **crypto/sha256** - File naming and content hashing
- **html/template** - Server-side HTML template rendering
- **encoding/json** - JSON serialization for item metadata
- **log/slog** - Structured logging (Go 1.21+ feature)

### Development Tools
- **go fmt** - Code formatting (Go standard)
- **go vet** - Static analysis tool
- **go build** - Compilation tool

---

## 🔄 Common Workflows

### Item Upload Workflow
1. User navigates to `/add` to access upload form
2. User fills in name (required), description (optional), and selects image file
3. Form submission to `/new` validates input and file size (max 1MB)
4. SHA256 hash is generated from item name for unique file naming
5. Image is saved to `uploads/` directory, metadata saved as JSON to `data/`
6. User is redirected to browse page to see all items

**Code path:** `/add` → `/new` → `handleNew()` → file system storage → redirect to `/`

### Item Browsing Workflow
1. User accesses root path `/` to view all items
2. Server loads all JSON files from `data/` directory
3. HTML template renders items with images and metadata
4. Static files are served from `/uploads/` path for images

**Code path:** `/` → `handleBrowse()` → `loadAllItems()` → template rendering

---

## 🚨 Things to Be Careful About

### 🔒 Security Considerations
- **File uploads:** Limited to 1MB size, but no file type validation beyond accept attribute
- **File naming:** SHA256 hashing prevents directory traversal attacks
- **Input validation:** Name and description length limits enforced

### ⚠️ Limitations
- **No database:** All data stored as individual JSON files
- **No authentication:** Open access to upload and browse functionality
- **No data persistence:** Files can be manually deleted from file system
- **Single server:** No clustering or load balancing support

*Updated at: 2025-01-27 UTC*