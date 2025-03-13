package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			_ = r.Body.Close() // fuck you
		}
		name := strings.Split(r.RequestURI, "/")
		if len(name) > 1 {
			fmt.Fprintf(w, "%s %s\n", name[1], os.Getenv("who"))
		}
	})
	srv := http.Server{
		Addr:              ":8080",
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		Handler:           mux,
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listen and serve", slog.Any("err", err))
		os.Exit(1)
	}
}
