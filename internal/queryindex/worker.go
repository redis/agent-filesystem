package queryindex

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Worker struct {
	RDB       *redis.Client
	FSKey     string
	BatchSize int
	Interval  time.Duration
	Logger    *log.Logger
}

func NewWorker(rdb *redis.Client, fsKey string) *Worker {
	return &Worker{
		RDB:       rdb,
		FSKey:     strings.TrimSpace(fsKey),
		BatchSize: defaultBatchSize,
		Interval:  defaultInterval,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.RDB == nil || strings.TrimSpace(w.FSKey) == "" {
		return nil
	}
	ok, err := EnsureIndex(ctx, w.RDB, w.FSKey)
	if err != nil {
		return err
	}
	if !ok {
		w.logf("query index disabled for %s: Redis Search unavailable", w.FSKey)
		<-ctx.Done()
		return ctx.Err()
	}
	_ = w.processOnce(ctx)
	interval := w.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_ = w.processOnce(ctx)
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) error {
	batchSize := w.BatchSize
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	result, err := ProcessPending(ctx, w.RDB, w.FSKey, batchSize)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		w.logf("query index batch failed for %s: %v", w.FSKey, err)
		return err
	}
	if result.Processed > 0 || result.Errors > 0 {
		w.logf("query index batch for %s: processed=%d indexed=%d skipped=%d deleted=%d errors=%d pending=%d",
			w.FSKey, result.Processed, result.Indexed, result.Skipped, result.Deleted, result.Errors, result.Pending)
	}
	return nil
}

func (w *Worker) logf(format string, args ...interface{}) {
	if w.Logger != nil {
		w.Logger.Printf(format, args...)
	}
}
