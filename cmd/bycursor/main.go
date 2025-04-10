package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	maxUploadSize        = 1024 * 1024 // 1 MB
	maxNameLength        = 120
	maxDescriptionLength = 160
	uploadDir            = "./uploads"
	dataDir              = "./data"
	serverAddr           = ":8080"
)

type Item struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ImagePath   string `json:"image_path,omitempty"`
}

type Server struct {
	logger *slog.Logger
	mu     sync.RWMutex
}

func main() {
	l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	ctx := context.Background()
	if err := run(ctx, l); err != nil {
		l.Error("failed to run server", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, l *slog.Logger) error {
	handleErr := func(err error) error {
		return fmt.Errorf("run: %w", err)
	}

	// Create required directories
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return handleErr(fmt.Errorf("failed to create upload directory: %w", err))
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return handleErr(fmt.Errorf("failed to create data directory: %w", err))
	}

	server := &Server{
		logger: l,
	}

	// Register handlers
	http.HandleFunc("/add", server.handleAdd)
	http.HandleFunc("/new", server.handleNew)
	http.HandleFunc("/", server.handleBrowse)
	// Serve static files from uploads directory
	http.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadDir))))

	l.Info("Starting server", "address", serverAddr)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		return handleErr(fmt.Errorf("failed to start server: %w", err))
	}

	return nil
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/add" {
		http.NotFound(w, r)
		return
	}

	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Upload Form</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
        }
        .form-group {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: bold;
        }
        input[type="text"], textarea {
            width: 100%;
            padding: 8px;
            box-sizing: border-box;
        }
        .required:after {
            content: " *";
            color: red;
        }
        button {
            background-color: #4CAF50;
            color: white;
            padding: 10px 15px;
            border: none;
            cursor: pointer;
        }
    </style>
</head>
<body>
    <h1>Upload Form</h1>
    <form action="/new" method="post" enctype="multipart/form-data">
        <div class="form-group">
            <label for="name" class="required">Name</label>
            <input type="text" id="name" name="name" maxlength="120" required>
        </div>
        <div class="form-group">
            <label for="description">Description</label>
            <textarea id="description" name="description" maxlength="160"></textarea>
        </div>
        <div class="form-group">
            <label for="image">Image (max 1MB)</label>
            <input type="file" id="image" name="image" accept="image/*">
        </div>
        <button type="submit">Submit</button>
    </form>
</body>
</html>
`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form with size limit
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		s.logger.Error("Failed to parse form", "error", err)
		http.Error(w, "The uploaded file is too big. Please choose a file that's less than 1MB.", http.StatusBadRequest)
		return
	}

	// Get form values
	name := r.FormValue("name")
	description := r.FormValue("description")

	// Validate name
	if name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if len(name) > maxNameLength {
		http.Error(w, fmt.Sprintf("Name must be at most %d characters", maxNameLength), http.StatusBadRequest)
		return
	}

	// Validate description
	if len(description) > maxDescriptionLength {
		http.Error(w, fmt.Sprintf("Description must be at most %d characters", maxDescriptionLength), http.StatusBadRequest)
		return
	}

	// Generate hash from name for file naming
	hash := sha256.Sum256([]byte(name))
	hashStr := hex.EncodeToString(hash[:])

	// Create item to store
	item := Item{
		Name:        name,
		Description: description,
	}

	// Handle file upload if present
	file, handler, err := r.FormFile("image")
	if err == nil {
		defer file.Close()

		// Check file size
		if handler.Size > maxUploadSize {
			http.Error(w, "The uploaded file is too big. Please choose a file that's less than 1MB.", http.StatusBadRequest)
			return
		}

		// Get file extension
		ext := filepath.Ext(handler.Filename)
		if ext == "" {
			ext = ".jpg" // Default extension
		}

		// Create filename with hash
		filename := hashStr + ext
		filePath := filepath.Join(uploadDir, filename)

		// Create destination file
		dst, err := os.Create(filePath)
		if err != nil {
			s.logger.Error("Failed to create file", "error", err)
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		// Copy file content
		if _, err := io.Copy(dst, file); err != nil {
			s.logger.Error("Failed to save file", "error", err)
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}

		// Set image path in item
		item.ImagePath = "/uploads/" + filename
	} else if err != http.ErrMissingFile {
		s.logger.Error("Failed to get file from form", "error", err)
		http.Error(w, "Failed to process file upload", http.StatusInternalServerError)
		return
	}

	// Save item to JSON file
	jsonPath := filepath.Join(dataDir, hashStr+".json")
	jsonData, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		s.logger.Error("Failed to marshal JSON", "error", err)
		http.Error(w, "Failed to save data", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		s.logger.Error("Failed to write JSON file", "error", err)
		http.Error(w, "Failed to save data", http.StatusInternalServerError)
		return
	}

	// Redirect to browse page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	items, err := s.loadAllItems()
	if err != nil {
		s.logger.Error("Failed to load items", "error", err)
		http.Error(w, "Failed to load items", http.StatusInternalServerError)
		return
	}

	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Browse Items</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 20px;
        }
        .item {
            border: 1px solid #ddd;
            padding: 15px;
            margin-bottom: 20px;
            border-radius: 5px;
            display: flex;
        }
        .item-image {
            flex: 0 0 150px;
            margin-right: 15px;
        }
        .item-image img {
            max-width: 100%;
            max-height: 150px;
        }
        .item-content {
            flex: 1;
        }
        .item-name {
            font-size: 18px;
            font-weight: bold;
            margin-bottom: 5px;
        }
        .item-description {
            color: #666;
        }
        .no-items {
            text-align: center;
            color: #666;
            padding: 30px;
        }
        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
        }
        .add-button {
            background-color: #4CAF50;
            color: white;
            padding: 10px 15px;
            text-decoration: none;
            border-radius: 4px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>Browse Items</h1>
        <a href="/add" class="add-button">Add New Item</a>
    </div>
    {{if .}}
        {{range .}}
        <div class="item">
            <div class="item-image">
                {{if .ImagePath}}
                <img src="{{.ImagePath}}" alt="{{.Name}}">
                {{else}}
                <div style="height: 150px; display: flex; align-items: center; justify-content: center; background-color: #f5f5f5;">
                    No Image
                </div>
                {{end}}
            </div>
            <div class="item-content">
                <div class="item-name">{{.Name}}</div>
                {{if .Description}}
                <div class="item-description">{{.Description}}</div>
                {{end}}
            </div>
        </div>
        {{end}}
    {{else}}
        <div class="no-items">No items found. <a href="/add">Add your first item</a></div>
    {{end}}
</body>
</html>
`

	tmpl, err := template.New("browse").Parse(html)
	if err != nil {
		s.logger.Error("Failed to parse template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, items); err != nil {
		s.logger.Error("Failed to execute template", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *Server) loadAllItems() ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var items []Item

	files, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read data directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dataDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			s.logger.Error("Failed to read file", "path", filePath, "error", err)
			continue
		}

		var item Item
		if err := json.Unmarshal(data, &item); err != nil {
			s.logger.Error("Failed to unmarshal JSON", "path", filePath, "error", err)
			continue
		}

		items = append(items, item)
	}

	return items, nil
}
