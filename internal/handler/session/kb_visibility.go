package session

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// filterVisibleKnowledgeBaseIDs drops explicitly requested knowledge bases the
// caller cannot see. The chat request carries knowledge_base_ids from the
// client, so without this check any authenticated member could ask questions
// against a private KB by guessing its ID. Candidates come from the same
// visibility filter as the console list (scope + kb_members + org share), so
// an invisible KB is treated as if it did not exist.
//
// API-key callers are handled earlier by AuthorizeTenantAPIKeyKnowledgeTargets
// and keep their own allow-list semantics; this guard only applies to users.
func (h *Handler) filterVisibleKnowledgeBaseIDs(ctx context.Context, ids []string) []string {
	if len(ids) == 0 {
		return ids
	}
	if _, ok := types.UserIDFromContext(ctx); !ok {
		return ids
	}
	if h.knowledgebaseService == nil {
		return ids
	}
	visible, err := h.knowledgebaseService.ListKnowledgeBases(ctx)
	if err != nil {
		logger.Warnf(ctx, "[qa] failed to resolve visible knowledge bases, rejecting explicit targets: %v", err)
		return nil
	}
	allowed := make(map[string]struct{}, len(visible))
	for _, kb := range visible {
		if kb != nil && kb.ID != "" {
			allowed[kb.ID] = struct{}{}
		}
	}
	out := make([]string, 0, len(ids))
	dropped := 0
	for _, id := range ids {
		if _, ok := allowed[id]; ok {
			out = append(out, id)
			continue
		}
		dropped++
	}
	if dropped > 0 {
		logger.Warnf(ctx, "[qa] dropped %d knowledge base id(s) not visible to the caller", dropped)
	}
	return out
}