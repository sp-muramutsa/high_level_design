// Package api provides HTTP request handlers, validation, and JSON responses.
package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter configures the Chi HTTP router with production middleware and API endpoints.
func NewRouter(h *Handler, port string) http.Handler {
	r := chi.NewRouter()

	// Standard middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	// CORS for React frontend (Vite dev and production)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:80", "http://localhost:8080", "*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Location", "X-Cache-Status", "X-Lookup-Latency-Us"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","port":"%s","time":"%s"}`+"\n", port, time.Now().UTC().Format(time.RFC3339))
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Post("/shorten", h.Shorten)
		r.Get("/stats/{code}", h.Stats)
		r.Post("/simulate-stampede", h.SimulateStampede)
	})

	// 302 Redirect route (supports both GET and HEAD)
	r.Get("/{code}", h.Redirect)
	r.Head("/{code}", h.Redirect)

	return r
}
