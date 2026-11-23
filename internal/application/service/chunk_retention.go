package service

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/runtime"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// ChunkRetentionRunner sweeps tombstoned chunks once a day. It mirrors
// AuditLogRetentionRunner: a bare time.Ticker, no cron, because retention only
// needs "approximately daily, eventually".
//
// Why this exists: deleting a document soft-deletes its chunks (GORM stamps
// deleted_at), and retrieval filters them out, so nothing ever reclaimed the
// space. Production reached 926k chunk rows / 1.1 GB with 94% of them dead and
// orphaned from any live document. The sweep hard-deletes rows whose
// deleted_at is older than the retention window, preserving a grace period for
// inspection and any in-flight reparse.
//
// retentionDays <= 0 disables the sweep (Start becomes a no-op); config
// validation rejects negative values, so 0 is always an explicit choice.
type ChunkRetentionRunner struct {
	repo          interfaces.ChunkRepository
	retentionDays int
	interval      time.Duration
	startupDelay  time.Duration
	// batchSize caps each DELETE statement. 5000 keeps a sweep to short
	// transactions; a backlog is drained over successive batches (and, if
	// needed, successive days) instead of one long lock.
	batchSize int

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
	started   atomic.Bool
}

const (
	chunkPurgeInterval     = 24 * time.Hour
	chunkPurgeStartupDelay = 10 * time.Minute
	chunkPurgeBatchSize    = 5000
	chunkPurgeTimeout      = 5 * time.Minute
)

// NewChunkRetentionRunner reads the retention window from the config. It only
// validates inputs; nothing fires until Start is called.
func NewChunkRetentionRunner(
	cfg *config.Config, repo interfaces.ChunkRepository,
) *ChunkRetentionRunner {
	retentionDays := config.DefaultDeletedChunkRetentionDays
	if cfg != nil && cfg.KnowledgeBase != nil {
		retentionDays = cfg.KnowledgeBase.EffectiveDeletedChunkRetentionDays()
	}
	return &ChunkRetentionRunner{
		repo:          repo,
		retentionDays: retentionDays,
		interval:      chunkPurgeInterval,
		startupDelay:  chunkStartupDelay(),
		batchSize:     chunkPurgeBatchSize,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

// Start spins up the background goroutine. Calling it more than once is a
// no-op; retentionDays <= 0 leaves the runner dormant.
func (r *ChunkRetentionRunner) Start(ctx context.Context) {
	if r == nil || r.repo == nil {
		return
	}
	r.startOnce.Do(func() {
		r.started.Store(true)
		if r.retentionDays <= 0 {
			logger.Infof(ctx,
				"[chunk-retention] disabled (deleted_chunk_retention_days=%d)", r.retentionDays)
			close(r.doneCh)
			return
		}
		logger.Infof(ctx,
			"[chunk-retention] starting daily sweep: retention_days=%d interval=%s",
			r.retentionDays, r.interval)
		go r.loop()
	})
}

// Stop signals the loop to exit and blocks until it returns. Idempotent.
func (r *ChunkRetentionRunner) Stop() {
	r.StopWithin(0)
}

// StopWithin is Stop with a bound. timeout <= 0 waits until the loop returns.
func (r *ChunkRetentionRunner) StopWithin(timeout time.Duration) {
	if r == nil {
		return
	}
	if !r.started.Load() {
		return
	}
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})
	if runtime.WaitFor(r.doneCh, timeout) {
		return
	}
	logger.Warnf(context.Background(),
		"[chunk-retention] sweep still running after %s; continuing shutdown", timeout)
}

func (r *ChunkRetentionRunner) loop() {
	defer close(r.doneCh)

	startupTimer := time.NewTimer(r.startupDelay)
	defer startupTimer.Stop()
	select {
	case <-startupTimer.C:
	case <-r.stopCh:
		return
	}

	r.runOnce()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.runOnce()
		case <-r.stopCh:
			return
		}
	}
}

// runOnce performs one sweep. The cutoff is computed fresh each time so a long
// uptime doesn't shrink the window; the DB call gets a bounded context so a
// stuck connection cannot hold shutdown hostage.
func (r *ChunkRetentionRunner) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), chunkPurgeTimeout)
	defer cancel()

	cutoff := time.Now().Add(-time.Duration(r.retentionDays) * 24 * time.Hour)
	deleted, err := r.repo.PurgeDeletedChunks(ctx, cutoff, r.batchSize)
	if err != nil {
		logger.Warnf(ctx,
			"[chunk-retention] sweep failed: retention_days=%d deleted=%d err=%v",
			r.retentionDays, deleted, err)
		return
	}
	if deleted > 0 {
		logger.Infof(ctx,
			"[chunk-retention] sweep complete: deleted=%d retention_days=%d cutoff=%s",
			deleted, r.retentionDays, cutoff.Format(time.RFC3339))
	} else {
		logger.Debugf(ctx,
			"[chunk-retention] sweep complete: deleted=0 retention_days=%d", r.retentionDays)
	}
}

// chunkStartupDelay lets an operator shorten the first sweep (for example right
// after a manual cleanup) without waiting the default ten minutes. Invalid or
// non-positive values fall back to the default so a typo cannot make the sweep
// fire during the startup request flood.
func chunkStartupDelay() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WEKNORA_CHUNK_RETENTION_START_DELAY"))
	if raw == "" {
		return chunkPurgeStartupDelay
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return chunkPurgeStartupDelay
	}
	return d
}
