package handler

import (
	"net/http"
	"sort"
	"strings"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

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

	hits := make([]kbSearchHit, 0, req.TopK)
	searched := 0
	for _, kb := range visible {
		if kb == nil || kb.ID == "" {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[kb.ID]; !ok {
				continue
			}
		}
		searched++
		perKB := perKBMatches(req.TopK)
		params := types.SearchParams{
			QueryText:  query,
			MatchCount: perKB,
			// Rerank gives a calibrated relevance score, which is what lets the
			// page say "没有找到" instead of returning weak vector neighbours.
			Rerank: &types.RerankOptions{TopK: perKB},
		}
		var results []*types.SearchResult
		retrieval, rerankErr := h.service.HybridSearchWithRerank(ctx, kb.ID, params)
		if rerankErr == nil && retrieval != nil {
			results = retrieval.Results
		} else {
			if rerankErr != nil {
				logger.Warnf(ctx, "[kb-search] rerank failed for kb %s, fallback to raw recall: %v", kb.ID, rerankErr)
			}
			results, rerankErr = h.service.HybridSearch(ctx, kb.ID, types.SearchParams{
				QueryText:  query,
				MatchCount: perKB,
			})
		}
		if rerankErr != nil {
			// A KB without a usable index must not fail the whole search.
			logger.Warnf(ctx, "[kb-search] skip kb %s: %v", kb.ID, rerankErr)
			continue
		}
		for _, hit := range results {
			if hit == nil {
				continue
			}
			hits = append(hits, kbSearchHit{
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
	}

	// Keep only hits that share at least one query token with the passage.
	// Fusion scores stay high on a single-domain corpus even for off-topic
	// queries, so the score alone cannot answer "查不到就说没有"; token
	// overlap is the cheap, deterministic relevance gate.
	tokens := queryTokens(query)
	if len(tokens) > 0 {
		kept := hits[:0]
		for _, hit := range hits {
			if containsAnyToken(hit.KnowledgeTitle+" "+hit.Content, tokens) {
				kept = append(kept, hit)
			}
		}
		hits = kept
	}

	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })

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
