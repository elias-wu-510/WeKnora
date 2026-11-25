package handler

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// maxKBConcurrency caps how many knowledge bases are queried at once. Rerank
// calls are the dominant cost, so the wall-clock time is roughly
// ceil(KBs/limit) model round-trips instead of one per KB. Four keeps the
// fan-out useful for a personal workspace while staying a small multiple of
// the per-tenant rerank capacity.
const maxKBConcurrency = 4

// kbSearchRequest is the body of the workspace-wide retrieval endpoint used by
// the C-Knowledge search page. knowledge_base_ids is optional: when omitted
// every knowledge base the caller can see is searched.
type kbSearchRequest struct {
	QueryText        string   `json:"query_text"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
	TopK             int      `json:"top_k"`
}

type kbSearchHit struct {
	KnowledgeBaseID    string  `json:"knowledge_base_id"`
	KnowledgeBaseName  string  `json:"knowledge_base_name"`
	KnowledgeID        string  `json:"knowledge_id"`
	KnowledgeTitle     string  `json:"knowledge_title"`
	Content            string  `json:"content"`
	Score              float64 `json:"score"`
	MatchType          string  `json:"match_type"`
	ChunkIndex         int     `json:"chunk_index"`
	KnowledgeBaseScope string  `json:"knowledge_base_scope"`
}

// SearchVisibleKnowledge runs one hybrid query across the caller's visible
// knowledge bases and returns a single ranked list. Visibility is resolved via
// ListKnowledgeBases (the same filter as the console list), so a KB the caller
// cannot see is silently dropped instead of being searched.
func (h *KnowledgeBaseHandler) SearchVisibleKnowledge(c *gin.Context) {
	ctx := c.Request.Context()

	var req kbSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	query := strings.TrimSpace(req.QueryText)
	if query == "" {
		_ = c.Error(apperrors.NewBadRequestError("query_text is required"))
		return
	}
	if req.TopK <= 0 || req.TopK > 50 {
		req.TopK = 20
	}

	visible, err := h.service.ListKnowledgeBases(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		_ = c.Error(apperrors.NewInternalServerError("Failed to list knowledge bases"))
		return
	}

	wanted := make(map[string]struct{}, len(req.KnowledgeBaseIDs))
	for _, id := range req.KnowledgeBaseIDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = struct{}{}
		}
	}

	targets := make([]*types.KnowledgeBase, 0, len(visible))
	for _, kb := range visible {
		if kb == nil || kb.ID == "" {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[kb.ID]; !ok {
				continue
			}
		}
		targets = append(targets, kb)
	}
	searched := len(targets)

	perKB := perKBMatches(req.TopK)
	perKBHits := make([][]kbSearchHit, len(targets))

	// errgroup is used purely as a bounded worker pool: each task contains its
	// own failure (logged, then skipped) and returns nil, so one unindexed KB
	// can never cancel the siblings that are still searching.
	limit := len(targets)
	if limit > maxKBConcurrency {
		limit = maxKBConcurrency
	}
	var group errgroup.Group
	if limit > 0 {
		group.SetLimit(limit)
	}
	for i, kb := range targets {
		group.Go(func() error {
			defer func() {
				if r := recover(); r != nil {
					// gin.Recovery only covers the request goroutine; without this
					// recover a panic in one KB's backend would kill the process
					// instead of just dropping that KB's rows.
					logger.Errorf(ctx, "[kb-search] kb %s panicked, skipping: %v", kb.ID, r)
				}
			}()
			// Each goroutine owns exactly one slot, so the merge below follows the
			// visible-KB order no matter which search finishes first.
			perKBHits[i] = searchKnowledgeBase(ctx, h.service, kb, query, perKB)
			return nil
		})
	}
	_ = group.Wait()

	hits := make([]kbSearchHit, 0, req.TopK)
	for _, kbHits := range perKBHits {
		hits = append(hits, kbHits...)
	}

	// Keep only hits that share at least one query token with the passage.
	// Fusion scores stay high on a single-domain corpus even for off-topic
	// queries, so the score alone cannot answer "查不到就说没有"; token
	// overlap is the cheap, deterministic relevance gate.
	//
	// A zero fusion score means neither the vector nor the keyword branch
	// matched anything: such rows only surface through incidental token
	// overlap (e.g. "such" inside an English passage) and must not be shown
	// as an answer to the user.
	tokens := queryTokens(query)
	kept := hits[:0]
	for _, hit := range hits {
		if hit.Score <= 0 {
			continue
		}
		if len(tokens) > 0 && !containsAnyToken(hit.KnowledgeTitle+" "+hit.Content, tokens) {
			continue
		}
		kept = append(kept, hit)
	}
	hits = kept

	// Total order, not just by score: KBs now finish in arbitrary order, so the
	// pre-sort order is nondeterministic. The dedupe below keeps the first row
	// of each document, which means ties would otherwise decide *which chunk*
	// is returned. (Score, KnowledgeID, ChunkIndex) makes that choice stable.
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].KnowledgeID != hits[j].KnowledgeID {
			return hits[i].KnowledgeID < hits[j].KnowledgeID
		}
		return hits[i].ChunkIndex < hits[j].ChunkIndex
	})

	// One row per document: the page is a result list, not a chunk dump.
	seen := make(map[string]struct{}, len(hits))
	deduped := hits[:0]
	for _, hit := range hits {
		if _, ok := seen[hit.KnowledgeID]; ok {
			continue
		}
		seen[hit.KnowledgeID] = struct{}{}
		deduped = append(deduped, hit)
		if len(deduped) >= req.TopK {
			break
		}
	}
	hits = deduped

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"query":             query,
			"results":           hits,
			"searched_kb_count": searched,
		},
	})
}

// searchKnowledgeBase runs the rerank-then-recall query for one knowledge base
// and maps the results onto the search page's row shape. A KB-level failure is
// logged and yields nil so the caller can skip that KB without failing the
// search.
func searchKnowledgeBase(
	ctx context.Context,
	svc interfaces.KnowledgeBaseService,
	kb *types.KnowledgeBase,
	query string,
	perKB int,
) []kbSearchHit {
	params := types.SearchParams{
		QueryText:  query,
		MatchCount: perKB,
		// Rerank gives a calibrated relevance score, which is what lets the
		// page say "没有找到" instead of returning weak vector neighbours.
		Rerank: &types.RerankOptions{TopK: perKB},
	}
	var results []*types.SearchResult
	retrieval, rerankErr := svc.HybridSearchWithRerank(ctx, kb.ID, params)
	if rerankErr == nil && retrieval != nil {
		results = retrieval.Results
	} else {
		if rerankErr != nil {
			logger.Warnf(ctx, "[kb-search] rerank failed for kb %s, fallback to raw recall: %v", kb.ID, rerankErr)
		}
		results, rerankErr = svc.HybridSearch(ctx, kb.ID, types.SearchParams{
			QueryText:  query,
			MatchCount: perKB,
		})
	}
	if rerankErr != nil {
		// A KB without a usable index must not fail the whole search.
		logger.Warnf(ctx, "[kb-search] skip kb %s: %v", kb.ID, rerankErr)
		return nil
	}

	out := make([]kbSearchHit, 0, len(results))
	for _, hit := range results {
		if hit == nil {
			continue
		}
		out = append(out, kbSearchHit{
			KnowledgeBaseID:    kb.ID,
			KnowledgeBaseName:  kb.Name,
			KnowledgeID:        hit.KnowledgeID,
			KnowledgeTitle:     hit.KnowledgeTitle,
			Content:            snippet(hit.Content, 600),
			Score:              hit.Score,
			MatchType:          matchTypeLabel(hit.MatchType),
			ChunkIndex:         hit.ChunkIndex,
			KnowledgeBaseScope: kb.NormalizedScope(),
		})
	}
	return out
}

func perKBMatches(topK int) int {
	per := topK / 3
	if per < 5 {
		per = 5
	}
	if per > 15 {
		per = 15
	}
	return per
}

func snippet(content string, limit int) string {
	content = strings.TrimSpace(content)
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit]) + "…"
}

// queryTokens splits a query into ASCII words and CJK bigrams. Both are used
// only as a presence check, never for ranking.
func queryTokens(query string) []string {
	lower := strings.ToLower(query)
	var tokens []string
	var word []rune
	var cjk []rune
	flushWord := func() {
		// ASCII words shorter than three letters ("no", "of") match too much
		// noise as substrings; two-character CJK bigrams below are precise
		// enough to keep.
		if len(word) >= 3 {
			tokens = append(tokens, string(word))
		}
		word = word[:0]
	}
	flushCJK := func() {
		if len(cjk) >= 2 {
			for i := 0; i+1 < len(cjk); i++ {
				tokens = append(tokens, string(cjk[i:i+2]))
			}
		}
		cjk = cjk[:0]
	}
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			flushCJK()
			word = append(word, r)
		case r >= 0x4e00 && r <= 0x9fff:
			flushWord()
			cjk = append(cjk, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return tokens
}

func containsAnyToken(text string, tokens []string) bool {
	lower := strings.ToLower(text)
	for _, token := range tokens {
		if isASCII(token) {
			if containsWord(lower, token) {
				return true
			}
			continue
		}
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// containsWord matches an ASCII token on word boundaries so "no" does not
// match "noise" and "plan" does not match "explain".
func containsWord(text, token string) bool {
	offset := 0
	for {
		index := strings.Index(text[offset:], token)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(token)
		if (start == 0 || !isWordRune(rune(text[start-1]))) && (end == len(text) || !isWordRune(rune(text[end]))) {
			return true
		}
		offset = start + 1
		if offset >= len(text) {
			return false
		}
	}
}

func isWordRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func matchTypeLabel(matchType types.MatchType) string {
	switch matchType {
	case types.MatchTypeEmbedding:
		return "embedding"
	case types.MatchTypeKeywords:
		return "keywords"
	case types.MatchTypeNearByChunk:
		return "nearby"
	case types.MatchTypeParentChunk:
		return "parent"
	case types.MatchTypeRelationChunk:
		return "relation"
	case types.MatchTypeGraph:
		return "graph"
	default:
		return "other"
	}
}
