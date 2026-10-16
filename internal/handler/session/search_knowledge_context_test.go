package session

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type contextSearchSessionService struct {
	interfaces.SessionService
	search func(context.Context) error
}

func (s *contextSearchSessionService) SearchKnowledge(
	ctx context.Context, _ []string, _ []string, _ []types.TagScope, _ string, _ *types.KnowledgeSearchOptions,
) (*types.RetrievalResult, error) {
	return &types.RetrievalResult{}, s.search(ctx)
}

func TestSearchKnowledge_PropagatesClientCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	started := make(chan struct{})
	stopped := make(chan error, 1)
	release := make(chan struct{})
	h := &Handler{sessionService: &contextSearchSessionService{
		search: func(ctx context.Context) error {
			close(started)
			select {
			case <-ctx.Done():
				stopped <- ctx.Err()
				return ctx.Err()
			case <-release:
				return nil
			}
		},
	}}
	r := gin.New()
	r.POST("/knowledge-search", h.SearchKnowledge)
	server := httptest.NewServer(r)
	defer server.Close()
	defer close(release) // Release the handler even if cancellation is broken.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/knowledge-search",
		strings.NewReader(`{"query":"safety","knowledge_base_ids":["kb-1"]}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	clientDone := make(chan error, 1)
	go func() {
		resp, err := server.Client().Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		clientDone <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("search did not start")
	}
	cancel()
	select {
	case err := <-clientDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("client did not stop after cancellation")
	}
	select {
	case err := <-stopped:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("search continued after the client canceled its request")
	}
}

func TestSearchKnowledge_PreservesRequestDeadlineAndIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(10000))
	ctx = context.WithValue(ctx, types.RequestIDContextKey, "search-context-test")

	var searchCtx context.Context
	h := &Handler{sessionService: &contextSearchSessionService{
		search: func(ctx context.Context) error {
			searchCtx = ctx
			return nil
		},
	}}
	r := gin.New()
	r.POST("/knowledge-search", h.SearchKnowledge)
	req := httptest.NewRequest(http.MethodPost, "/knowledge-search",
		strings.NewReader(`{"query":"safety","knowledge_base_ids":["kb-1"]}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, searchCtx)
	gotDeadline, ok := searchCtx.Deadline()
	require.True(t, ok, "search lost the request deadline")
	require.Equal(t, deadline, gotDeadline)
	require.Equal(t, uint64(10000), searchCtx.Value(types.TenantIDContextKey))
	require.Equal(t, "search-context-test", searchCtx.Value(types.RequestIDContextKey))
}
