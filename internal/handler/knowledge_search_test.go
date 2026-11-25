package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

func TestQueryTokens(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"建筑高度限制", []string{"建筑", "筑高", "高度", "度限", "限制"}},
		{"OTTV 计算", []string{"ottv", "计算"}},
		{"zzzz-no-such-topic", []string{"zzzz", "such", "topic"}},
		{"a", nil},
	}
	for _, tc := range cases {
		got := queryTokens(tc.query)
		if len(got) != len(tc.want) {
			t.Fatalf("query %q: got %v want %v", tc.query, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("query %q: got %v want %v", tc.query, got, tc.want)
			}
		}
	}
}

func TestContainsAnyToken(t *testing.T) {
	tokens := queryTokens("建筑高度限制")
	if !containsAnyToken("本规范规定建筑高度不得超过…", tokens) {
		t.Fatal("expected match on 建筑")
	}
	if containsAnyToken("今天天气不错，适合户外活动。", tokens) {
		t.Fatal("unexpected match for off-topic text")
	}
	if containsAnyToken("This noise report explains nothing.", queryTokens("no plan")) {
		t.Fatal("short ASCII token must not match as a substring")
	}
}

func TestPerKBMatchesBounds(t *testing.T) {
	if got := perKBMatches(20); got != 6 {
		t.Fatalf("perKBMatches(20) = %d, want 6", got)
	}
	if got := perKBMatches(3); got != 5 {
		t.Fatalf("perKBMatches(3) = %d, want 5", got)
	}
	if got := perKBMatches(100); got != 15 {
		t.Fatalf("perKBMatches(100) = %d, want 15", got)
	}
}

// visibleSearchStub implements only the three service calls
// SearchVisibleKnowledge makes; embedding the interface keeps the rest out of
// the test. Behaviour is keyed by KB ID so table cases can mix healthy, failing
// and panicking KBs in one search.
type visibleSearchStub struct {
	interfaces.KnowledgeBaseService

	kbs []*types.KnowledgeBase

	// rerank / recall results and errors, keyed by KB ID.
	rerankResults map[string][]*types.SearchResult
	recallResults map[string][]*types.SearchResult
	rerankErr     map[string]error
	recallErr     map[string]error
	panicOn       map[string]bool
	delay         map[string]time.Duration

	mu       sync.Mutex
	inFlight int
	maxIn    int
	calls    []string
}

func (s *visibleSearchStub) ListKnowledgeBases(context.Context) ([]*types.KnowledgeBase, error) {
	return s.kbs, nil
}

func (s *visibleSearchStub) HybridSearchWithRerank(
	_ context.Context, kbID string, _ types.SearchParams,
) (*types.RetrievalResult, error) {
	s.begin(kbID)
	defer s.end()
	if d := s.delay[kbID]; d > 0 {
		time.Sleep(d)
	}
	if s.panicOn[kbID] {
		panic("stub: rerank backend exploded")
	}
	if err := s.rerankErr[kbID]; err != nil {
		return nil, err
	}
	return &types.RetrievalResult{Results: s.rerankResults[kbID]}, nil
}

func (s *visibleSearchStub) HybridSearch(
	_ context.Context, kbID string, _ types.SearchParams,
) ([]*types.SearchResult, error) {
	s.begin(kbID)
	defer s.end()
	if d := s.delay[kbID]; d > 0 {
		time.Sleep(d)
	}
	if err := s.recallErr[kbID]; err != nil {
		return nil, err
	}
	return s.recallResults[kbID], nil
}

func (s *visibleSearchStub) begin(kbID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight++
	if s.inFlight > s.maxIn {
		s.maxIn = s.inFlight
	}
	s.calls = append(s.calls, kbID)
}

func (s *visibleSearchStub) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight--
}

type visibleSearchEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Query           string        `json:"query"`
		Results         []kbSearchHit `json:"results"`
		SearchedKBCount int           `json:"searched_kb_count"`
	} `json:"data"`
}

func newVisibleSearchTestRouter(svc interfaces.KnowledgeBaseService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &KnowledgeBaseHandler{service: svc}
	router.POST("/knowledge-search/visible", handler.SearchVisibleKnowledge)
	return router
}

func performVisibleSearch(t *testing.T, svc interfaces.KnowledgeBaseService, req kbSearchRequest) visibleSearchEnvelope {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/knowledge-search/visible", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	newVisibleSearchTestRouter(svc).ServeHTTP(recorder, httpReq)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope visibleSearchEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, recorder.Body.String())
	}
	return envelope
}

func testKB(id string) *types.KnowledgeBase {
	return &types.KnowledgeBase{ID: id, Name: id + "-name"}
}

func testHit(knowledgeID string, chunk int, score float64) *types.SearchResult {
	return &types.SearchResult{
		KnowledgeID:    knowledgeID,
		KnowledgeTitle: "建筑高度规范",
		Content:        "建筑高度限制说明 " + knowledgeID,
		ChunkIndex:     chunk,
		Score:          score,
	}
}

func resultIDs(envelope visibleSearchEnvelope) []string {
	ids := make([]string, 0, len(envelope.Data.Results))
	for _, hit := range envelope.Data.Results {
		ids = append(ids, hit.KnowledgeID)
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSearchVisibleKnowledgeConcurrent(t *testing.T) {
	tests := []struct {
		name       string
		kbs        []*types.KnowledgeBase
		wanted     []string
		topK       int
		rerank     map[string][]*types.SearchResult
		recall     map[string][]*types.SearchResult
		rerankErr  map[string]error
		recallErr  map[string]error
		panicOn    map[string]bool
		wantIDs    []string
		wantSearch int
		wantChunks map[string]int
	}{
		{
			name: "results are merged across kbs and ranked by score, not completion order",
			kbs:  []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b"), testKB("kb-c")},
			topK: 20,
			rerank: map[string][]*types.SearchResult{
				// kb-a and kb-c are slow, kb-b is instant: completion order is
				// b,c,a while score order is b,c,a reversed. The response must
				// follow the scores.
				"kb-a": {testHit("doc-a", 0, 0.2)},
				"kb-b": {testHit("doc-b", 0, 0.9)},
				"kb-c": {testHit("doc-c", 0, 0.5)},
			},
			panicOn:    map[string]bool{},
			wantIDs:    []string{"doc-b", "doc-c", "doc-a"},
			wantSearch: 3,
		},
		{
			name: "one failing kb is skipped without failing the search",
			kbs:  []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b"), testKB("kb-c")},
			topK: 20,
			rerank: map[string][]*types.SearchResult{
				"kb-a": {testHit("doc-a", 0, 0.4)},
				"kb-c": {testHit("doc-c", 0, 0.8)},
			},
			rerankErr:  map[string]error{"kb-b": errors.New("no index")},
			recallErr:  map[string]error{"kb-b": errors.New("no index")},
			wantIDs:    []string{"doc-c", "doc-a"},
			wantSearch: 3,
		},
		{
			name: "one panicking kb is skipped without failing the search",
			kbs:  []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b"), testKB("kb-c")},
			topK: 20,
			rerank: map[string][]*types.SearchResult{
				"kb-a": {testHit("doc-a", 0, 0.4)},
				"kb-b": {testHit("doc-b", 0, 0.99)},
				"kb-c": {testHit("doc-c", 0, 0.8)},
			},
			panicOn:    map[string]bool{"kb-b": true},
			wantIDs:    []string{"doc-c", "doc-a"},
			wantSearch: 3,
		},
		{
			name: "rerank failure falls back to raw recall for that kb only",
			kbs:  []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b")},
			topK: 20,
			rerank: map[string][]*types.SearchResult{
				"kb-a": {testHit("doc-a", 0, 0.4)},
			},
			recall: map[string][]*types.SearchResult{
				"kb-b": {testHit("doc-b", 0, 0.7)},
			},
			rerankErr:  map[string]error{"kb-b": errors.New("rerank unavailable")},
			wantIDs:    []string{"doc-b", "doc-a"},
			wantSearch: 2,
		},
		{
			name:   "requested knowledge base ids still narrow the searched set",
			kbs:    []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b"), testKB("kb-c")},
			wanted: []string{"kb-a", "kb-c"},
			topK:   20,
			rerank: map[string][]*types.SearchResult{
				"kb-a": {testHit("doc-a", 0, 0.4)},
				"kb-b": {testHit("doc-b", 0, 0.9)},
				"kb-c": {testHit("doc-c", 0, 0.8)},
			},
			wantIDs:    []string{"doc-c", "doc-a"},
			wantSearch: 2,
		},
		{
			name: "top_k still truncates after dedupe",
			kbs:  []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b"), testKB("kb-c")},
			topK: 2,
			rerank: map[string][]*types.SearchResult{
				"kb-a": {testHit("doc-a", 0, 0.4)},
				"kb-b": {testHit("doc-b", 0, 0.9)},
				"kb-c": {testHit("doc-c", 0, 0.8)},
			},
			wantIDs:    []string{"doc-b", "doc-c"},
			wantSearch: 3,
		},
		{
			name: "one row per document keeps the lowest chunk index among equal scores",
			kbs:  []*types.KnowledgeBase{testKB("kb-a")},
			topK: 20,
			rerank: map[string][]*types.SearchResult{
				"kb-a": {
					testHit("doc-a", 7, 0.9),
					testHit("doc-a", 2, 0.9),
					testHit("doc-a", 4, 0.9),
				},
			},
			wantIDs:    []string{"doc-a"},
			wantSearch: 1,
			wantChunks: map[string]int{"doc-a": 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &visibleSearchStub{
				kbs:           tc.kbs,
				rerankResults: tc.rerank,
				recallResults: tc.recall,
				rerankErr:     tc.rerankErr,
				recallErr:     tc.recallErr,
				panicOn:       tc.panicOn,
			}
			envelope := performVisibleSearch(t, svc, kbSearchRequest{
				QueryText:        "建筑高度",
				KnowledgeBaseIDs: tc.wanted,
				TopK:             tc.topK,
			})

			if got := resultIDs(envelope); !equalStrings(got, tc.wantIDs) {
				t.Fatalf("result ids = %v, want %v (body results %+v)", got, tc.wantIDs, envelope.Data.Results)
			}
			if envelope.Data.SearchedKBCount != tc.wantSearch {
				t.Fatalf("searched_kb_count = %d, want %d", envelope.Data.SearchedKBCount, tc.wantSearch)
			}
			for knowledgeID, wantChunk := range tc.wantChunks {
				for _, hit := range envelope.Data.Results {
					if hit.KnowledgeID == knowledgeID && hit.ChunkIndex != wantChunk {
						t.Fatalf("doc %s kept chunk %d, want %d", knowledgeID, hit.ChunkIndex, wantChunk)
					}
				}
			}
			if !envelope.Success || envelope.Data.Query != "建筑高度" {
				t.Fatalf("unexpected envelope: %+v", envelope)
			}
		})
	}
}

// TestSearchVisibleKnowledgeTieOrderStable runs the same equal-score search many
// times: KB goroutines finish in varying order, so any nondeterminism in the
// merge/sort would flip rows, or even flip which chunk dedupe keeps.
func TestSearchVisibleKnowledgeTieOrderStable(t *testing.T) {
	svc := &visibleSearchStub{
		kbs: []*types.KnowledgeBase{testKB("kb-a"), testKB("kb-b"), testKB("kb-c")},
		rerankResults: map[string][]*types.SearchResult{
			"kb-a": {
				testHit("doc-c", 0, 0.5),
				testHit("doc-b", 4, 0.5),
				testHit("doc-b", 2, 0.5),
			},
			"kb-b": {testHit("doc-a", 0, 0.5)},
			"kb-c": {testHit("doc-d", 0, 0.5)},
		},
		delay: map[string]time.Duration{
			"kb-a": 3 * time.Millisecond,
			"kb-b": time.Millisecond,
		},
	}

	var first string
	for i := 0; i < 25; i++ {
		envelope := performVisibleSearch(t, svc, kbSearchRequest{QueryText: "建筑高度", TopK: 20})
		raw, err := json.Marshal(envelope.Data.Results)
		if err != nil {
			t.Fatalf("marshal results: %v", err)
		}
		if i == 0 {
			first = string(raw)
		} else if string(raw) != first {
			t.Fatalf("run %d returned a different order:\n got %s\nwant %s", i, raw, first)
		}
	}

	var results []kbSearchHit
	if err := json.Unmarshal([]byte(first), &results); err != nil {
		t.Fatalf("unmarshal first run: %v", err)
	}
	gotIDs := make([]string, 0, len(results))
	for _, hit := range results {
		gotIDs = append(gotIDs, hit.KnowledgeID)
	}
	if want := []string{"doc-a", "doc-b", "doc-c", "doc-d"}; !equalStrings(gotIDs, want) {
		t.Fatalf("tie order = %v, want %v", gotIDs, want)
	}
	for _, hit := range results {
		if hit.KnowledgeID == "doc-b" && hit.ChunkIndex != 2 {
			t.Fatalf("doc-b kept chunk %d, want 2 (lowest chunk index on a tie)", hit.ChunkIndex)
		}
	}
}

// TestSearchVisibleKnowledgeConcurrencyBound pins the worker-pool cap: many KBs
// must be searched in parallel, but never more than maxKBConcurrency at once.
func TestSearchVisibleKnowledgeConcurrencyBound(t *testing.T) {
	kbs := make([]*types.KnowledgeBase, 0, 10)
	results := make(map[string][]*types.SearchResult, 10)
	for i := 0; i < 10; i++ {
		id := "kb-" + strings.Repeat("x", i) // distinct, stable IDs
		kbs = append(kbs, testKB(id))
		results[id] = []*types.SearchResult{testHit("doc-"+id, 0, 0.5)}
	}
	svc := &visibleSearchStub{
		kbs:           kbs,
		rerankResults: results,
		delay:         map[string]time.Duration{},
	}
	for _, kb := range kbs {
		svc.delay[kb.ID] = 5 * time.Millisecond
	}

	envelope := performVisibleSearch(t, svc, kbSearchRequest{QueryText: "建筑高度", TopK: 20})

	if len(envelope.Data.Results) != 10 {
		t.Fatalf("got %d results, want 10", len(envelope.Data.Results))
	}
	svc.mu.Lock()
	maxIn := svc.maxIn
	svc.mu.Unlock()
	if maxIn > maxKBConcurrency {
		t.Fatalf("observed %d concurrent KB searches, cap is %d", maxIn, maxKBConcurrency)
	}
	if maxIn < 2 {
		t.Fatalf("observed no parallelism (max in flight %d)", maxIn)
	}
}
