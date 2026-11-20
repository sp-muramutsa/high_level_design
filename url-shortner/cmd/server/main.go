// Package main is the entrypoint for the URL Shortener service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"url-shortener/internal/analytics"
	"url-shortener/internal/api"
	"url-shortener/internal/cache"
	"url-shortener/internal/config"
	"url-shortener/internal/generator"
	"url-shortener/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server terminated with error: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log.Printf("Starting URL Shortener service on port %s...", cfg.Port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Initialize Redis Client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()

	if pingErr := rdb.Ping(ctx).Err(); pingErr != nil {
		log.Printf("[WARN] Redis not reachable at %s: %v (will retry on demand)", cfg.RedisAddr, pingErr)
	} else {
		log.Printf("Connected to Redis at %s", cfg.RedisAddr)
	}

	// 2. Initialize PostgreSQL Store
	var store storage.Store
	pgStore, pgErr := storage.NewPostgresStore(ctx, cfg.DatabaseURL)
	if pgErr != nil {
		log.Printf("[WARN] Postgres not reachable at %s: %v", cfg.DatabaseURL, pgErr)
	} else {
		defer pgStore.Close()
		store = pgStore
		log.Println("Connected to PostgreSQL successfully")
	}

	// 3. Initialize Range Allocator (Redis backed)
	allocator := generator.NewRedisRangeAllocator(rdb, "url_shortener:global_id_counter", cfg.IDChunkSize)

	// 4. Initialize Two-Tier Cache with Singleflight
	var dbFallback cache.DBFallbackFunc
	if store != nil {
		dbFallback = store.GetURLByShortCode
	} else {
		dbFallback = func(ctx context.Context, shortCode string) (*storage.URL, error) {
			return nil, storage.ErrNotFound
		}
	}

	twoTierCache := cache.NewTwoTierCache(rdb, dbFallback, 5000, 5*time.Second, cfg.CacheTTL)

	// 5. Initialize & Start Asynchronous Analytics Worker
	worker := analytics.NewWorker(rdb, store, 100, 2*time.Second)
	worker.Start(ctx)
	defer worker.Stop()

	// 6. Setup HTTP Router & Handlers
	handler := api.NewHandler(store, twoTierCache, allocator, rdb, cfg.BaseURL)
	router := api.NewRouter(handler, cfg.Port)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on :%s (Base URL: %s)", cfg.Port, cfg.BaseURL)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		log.Println("Shutdown signal received, shutting down gracefully...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	log.Println("Server stopped cleanly")
	return nil
}
