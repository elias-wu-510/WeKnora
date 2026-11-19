package service

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/hibiken/asynq"
)

func documentProcessTaskOptions(cfg *config.Config, extra ...asynq.Option) []asynq.Option {
	opts := []asynq.Option{
		asynq.Queue(types.QueueDefault),
		asynq.Timeout(config.DocumentProcessTimeout(cfg)),
		asynq.MaxRetry(3),
	}
	opts = append(opts, extra...)
	return opts
}

func knowledgePostProcessTaskOptions() []asynq.Option {
	return []asynq.Option{
		asynq.Queue(types.QueuePostProcess),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Minute),
	}
}

// applyDocumentTaskCorrelation ties an async document task to the HTTP
// request that enqueued it. The request id is copied into the payload so
// the worker (and docreader, which receives RequestID) logs the same
// correlation id as the upload/reparse call. Without it, parse failures
// can only be found by timestamp, not by the id shown to the user.
func applyDocumentTaskCorrelation(ctx context.Context, payload *types.DocumentProcessPayload) {
	if payload == nil {
		return
	}
	if requestID, ok := types.RequestIDFromContext(ctx); ok {
		payload.RequestId = requestID
	}
	langfuse.InjectTracing(ctx, payload)
}
