// Package analytics provides decoupled, asynchronous event queuing and batch flushing
// to keep database writes completely off the critical redirect path.
package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"url-shortener/internal/storage"
)

const (
	// DefaultQueueKey is the Redis list used to buffer incoming click events.
	DefaultQueueKey = "url_shortener:click_queue"
	// DefaultBatchSize is the maximum number of events flushed to PostgreSQL in a single transaction.
	DefaultBatchSize = 100
	// DefaultFlushInterval is the maximum duration to wait before flushing pending events.
	DefaultFlushInterval = 2 * time.Second
)

// EnqueueClick pushes a lightweight click event to the Redis queue in < 1ms.
// If Redis is temporarily unreachable, it fails gracefully without blocking the HTTP redirect.
func EnqueueClick(ctx context.Context, rdb *redis.Client, event *storage.ClickEvent) error {
	if rdb == nil {
		return errors.New("redis client is nil")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("analytics: marshal click event: %w", err)
	}

	if err := rdb.LPush(ctx, DefaultQueueKey, data).Err(); err != nil {
		return fmt.Errorf("analytics: lpush to redis queue: %w", err)
	}

	return nil
}

// Worker drains click events from Redis in batches and commits them to PostgreSQL.
type Worker struct {
	rdb           *redis.Client
	store         storage.Store
	queueKey      string
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewWorker constructs a new batch analytics flusher worker.
func NewWorker(rdb *redis.Client, store storage.Store, batchSize int, flushInterval time.Duration) *Worker {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = DefaultFlushInterval
	}
	return &Worker{
		rdb:           rdb,
		store:         store,
		queueKey:      DefaultQueueKey,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the background drain-and-flush worker goroutine.
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
}

// Stop signals the worker to finish flushing remaining in-flight events and shuts down.
func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()
	log.Printf("Analytics worker started (batchSize=%d, interval=%v)", w.batchSize, w.flushInterval)

	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	var batch []*storage.ClickEvent

	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, draining final analytics batch...")
			w.flushBatch(context.Background(), batch)
			return
		case <-w.stopCh:
			log.Println("Stopping analytics worker, draining remaining events...")
			w.flushBatch(context.Background(), batch)
			w.drainRemaining(context.Background())
			return
		case <-ticker.C:
			if len(batch) > 0 {
				w.flushBatch(ctx, batch)
				batch = batch[:0]
			}
		default:
			// Read up to batchSize items from Redis queue
			item, err := w.popEvent(ctx)
			if err != nil {
				// No items or connection pause, sleep briefly to prevent busy spinning
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if item != nil {
				batch = append(batch, item)
				if len(batch) >= w.batchSize {
					w.flushBatch(ctx, batch)
					batch = batch[:0]
				}
			}
		}
	}
}

func (w *Worker) popEvent(ctx context.Context) (*storage.ClickEvent, error) {
	if w.rdb == nil {
		return nil, errors.New("redis client unavailable")
	}

	data, err := w.rdb.RPop(ctx, w.queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Queue is empty
		}
		return nil, err
	}

	var event storage.ClickEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}

	return &event, nil
}

func (w *Worker) flushBatch(ctx context.Context, batch []*storage.ClickEvent) {
	if len(batch) == 0 || w.store == nil {
		return
	}

	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := w.store.SaveClickEventsBatch(flushCtx, batch); err != nil {
		log.Printf("[ERROR] Failed to batch-insert %d click events to PostgreSQL: %v", len(batch), err)
		return
	}

	log.Printf("[ANALYTICS] Flushed batch of %d click events to database", len(batch))
}

func (w *Worker) drainRemaining(ctx context.Context) {
	var remaining []*storage.ClickEvent
	for i := 0; i < w.batchSize*5; i++ {
		event, err := w.popEvent(ctx)
		if err != nil || event == nil {
			break
		}
		remaining = append(remaining, event)
	}
	if len(remaining) > 0 {
		w.flushBatch(ctx, remaining)
	}
}
