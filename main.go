package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"time"

	"openvoice/internal/database"
)

const (
	defaultAddr = ":8080"
	dbPath      = "data/openvoice.db"
)

//go:embed cmd/server/dist/*
var embeddedDist embed.FS

type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

func main() {
	db, err := database.InitDB(dbPath)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	distFS, err := fs.Sub(embeddedDist, "cmd/server/dist")
	if err != nil {
		log.Fatalf("frontend assets unavailable: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthHandler(db))
	mux.Handle("/", spaHandler(distFS))

	srv := &http.Server{
		Addr:         defaultAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("openvoice server listening on %s", defaultAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped unexpectedly: %v", err)
	}
}

func healthHandler(db dbPinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "connected"
		if err := db.PingContext(ctx); err != nil {
			dbStatus = "disconnected"
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(healthResponse{Status: "ok", DB: dbStatus}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	}
}

type dbPinger interface {
	PingContext(context.Context) error
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			http.NotFound(w, r)
			return
		}

		requested := r.URL.Path
		if requested == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		cleanPath := requested[1:]
		if _, err := fs.Stat(staticFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
