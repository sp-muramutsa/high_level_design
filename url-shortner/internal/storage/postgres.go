// Package storage provides data access and persistence operations with PostgreSQL.
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when a requested URL or record does not exist.
	ErrNotFound = errors.New("record not found")
	// ErrAlreadyExists is returned when attempting to insert a duplicate short code.
	ErrAlreadyExists = errors.New("short code already exists")
)

// URL represents a shortened URL mapping.
type URL struct {
	ID          uint64     `json:"id"`
	ShortCode   string     `json:"short_code"`
	OriginalURL string     `json:"original_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	IsCustom    bool       `json:"is_custom"`
}

// ClickEvent captures metadata for an asynchronous redirect visit.
type ClickEvent struct {
	ID        uint64    `json:"id"`
	ShortCode string    `json:"short_code"`
	ClickedAt time.Time `json:"clicked_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Referrer  string    `json:"referrer"`
}

// Stats aggregates engagement metrics for a given short code.
type Stats struct {
	ShortCode   string    `json:"short_code"`
	OriginalURL string    `json:"original_url"`
	CreatedAt   time.Time `json:"created_at"`
	TotalClicks int64     `json:"total_clicks"`
	LastClicked *time.Time `json:"last_clicked,omitempty"`
}

// Store defines persistent storage operations for URLs and analytics.
type Store interface {
	SaveURL(ctx context.Context, u *URL) error
	GetURLByShortCode(ctx context.Context, shortCode string) (*URL, error)
	SaveClickEventsBatch(ctx context.Context, events []*ClickEvent) error
	GetStats(ctx context.Context, shortCode string) (*Stats, error)
	Close()
}

// PostgresStore implements Store using a pgx connection pool.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new connected PostgresStore with a tuned connection pool.
func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("storage: parse config: %w", err)
	}

	cfg.MaxConns = 30
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 1 * time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("storage: connect to database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("storage: ping database: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

// Close closes the underlying connection pool.
func (s *PostgresStore) Close() {
	s.pool.Close()
}

// SaveURL inserts a new URL mapping. If the short_code collides with an existing
// record, it returns ErrAlreadyExists.
func (s *PostgresStore) SaveURL(ctx context.Context, u *URL) error {
	query := `
		INSERT INTO urls (id, short_code, original_url, created_at, expires_at, is_custom)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.pool.Exec(ctx, query,
		u.ID,
		u.ShortCode,
		u.OriginalURL,
		u.CreatedAt,
		u.ExpiresAt,
		u.IsCustom,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrAlreadyExists
		}
		return fmt.Errorf("storage: save url: %w", err)
	}
	return nil
}

// GetURLByShortCode retrieves a URL mapping by its short code.
func (s *PostgresStore) GetURLByShortCode(ctx context.Context, shortCode string) (*URL, error) {
	query := `
		SELECT id, short_code, original_url, created_at, expires_at, is_custom
		FROM urls
		WHERE short_code = $1
	`
	row := s.pool.QueryRow(ctx, query, shortCode)

	var u URL
	err := row.Scan(
		&u.ID,
		&u.ShortCode,
		&u.OriginalURL,
		&u.CreatedAt,
		&u.ExpiresAt,
		&u.IsCustom,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: get url: %w", err)
	}

	return &u, nil
}

// SaveClickEventsBatch inserts a slice of ClickEvents in a single batch transaction.
func (s *PostgresStore) SaveClickEventsBatch(ctx context.Context, events []*ClickEvent) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO click_events (short_code, clicked_at, ip_address, user_agent, referrer)
		VALUES ($1, $2, $3, $4, $5)
	`
	for _, e := range events {
		batch.Queue(query, e.ShortCode, e.ClickedAt, e.IPAddress, e.UserAgent, e.Referrer)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(events); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("storage: batch insert click event: %w", err)
		}
	}

	return nil
}

// GetStats queries total clicks and latest click timestamp for a short URL.
func (s *PostgresStore) GetStats(ctx context.Context, shortCode string) (*Stats, error) {
	u, err := s.GetURLByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT COUNT(*), MAX(clicked_at)
		FROM click_events
		WHERE short_code = $1
	`
	var totalClicks int64
	var lastClicked *time.Time

	err = s.pool.QueryRow(ctx, query, shortCode).Scan(&totalClicks, &lastClicked)
	if err != nil {
		return nil, fmt.Errorf("storage: get stats: %w", err)
	}

	return &Stats{
		ShortCode:   u.ShortCode,
		OriginalURL: u.OriginalURL,
		CreatedAt:   u.CreatedAt,
		TotalClicks: totalClicks,
		LastClicked: lastClicked,
	}, nil
}
