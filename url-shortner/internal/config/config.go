// Package config provides configuration loading from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration parameters for the URL shortener service.
type Config struct {
	// Server
	Port            string
	BaseURL         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration

	// Database
	DatabaseURL string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// ID Allocator
	IDChunkSize uint64

	// Cache TTL
	CacheTTL time.Duration
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	port := getEnv("PORT", "8080")
	baseURL := getEnv("BASE_URL", fmt.Sprintf("http://localhost:%s", port))

	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/urlshortener?sslmode=disable")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	redisPassword := os.Getenv("REDIS_PASSWORD")

	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	chunkSize, err := strconv.ParseUint(getEnv("ID_CHUNK_SIZE", "1000"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ID_CHUNK_SIZE: %w", err)
	}

	cacheTTLSeconds, err := strconv.Atoi(getEnv("CACHE_TTL_SECONDS", "86400"))
	if err != nil {
		return nil, fmt.Errorf("invalid CACHE_TTL_SECONDS: %w", err)
	}

	return &Config{
		Port:            port,
		BaseURL:         baseURL,
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		DatabaseURL:     dbURL,
		RedisAddr:       redisAddr,
		RedisPassword:   redisPassword,
		RedisDB:         redisDB,
		IDChunkSize:     chunkSize,
		CacheTTL:        time.Duration(cacheTTLSeconds) * time.Second,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}
