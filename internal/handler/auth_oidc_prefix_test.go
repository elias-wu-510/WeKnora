package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/gin-gonic/gin"
)

func newPrefixTestContext(prefix string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback", nil)
	c.Request.Host = "llm-ai.example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	if prefix != "" {
		c.Request.Header.Set("X-Forwarded-Prefix", prefix)
	}
	return c, w
}

func TestWebBasePath(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		want   string
	}{
		{"root deployment", "", ""},
		{"explicit root", "/", ""},
		{"sub-path", "/c-knowledge", "/c-knowledge"},
		{"sub-path with trailing slash", "/c-knowledge/", "/c-knowledge"},
		{"no leading slash", "c-knowledge", ""},
		{"path traversal", "/c-knowledge/../admin", ""},
		{"query or fragment", "/c-knowledge?x=1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newPrefixTestContext(tc.prefix)
			if got := webBasePath(c); got != tc.want {
				t.Fatalf("webBasePath(%q) = %q, want %q", tc.prefix, got, tc.want)
			}
		})
	}
}

// TestOIDCCallbackURLKeepsPrefix guards the deep-link flow used by enterprise
// portals: /auth/oidc/start must send the IdP a callback URL that still points
// at the prefixed deployment, otherwise the provider returns to the site root.
func TestOIDCCallbackURLKeepsPrefix(t *testing.T) {
	c, _ := newPrefixTestContext("/c-knowledge")
	want := "https://llm-ai.example.com/c-knowledge/api/v1/auth/oidc/callback"
	if got := oidcCallbackURL(c); got != want {
		t.Fatalf("oidcCallbackURL = %q, want %q", got, want)
	}

	c, _ = newPrefixTestContext("")
	want = "https://llm-ai.example.com/api/v1/auth/oidc/callback"
	if got := oidcCallbackURL(c); got != want {
		t.Fatalf("root oidcCallbackURL = %q, want %q", got, want)
	}
}

// TestOIDCRedirectCallbackLandsInsidePrefix: the SPA stores the auth payload in
// the URL hash, so bouncing to "/" instead of "/c-knowledge/" loses the login.
func TestOIDCRedirectCallbackLandsInsidePrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(errorCapture())
	h := NewAuthHandler(&config.Config{}, nil, nil, nil, nil)
	r.GET("/api/v1/auth/oidc/callback", h.OIDCRedirectCallback)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?state=not-a-real-state", nil)
	req.Host = "llm-ai.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Prefix", "/c-knowledge")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if len(loc) < len("/c-knowledge/#oidc_error=") || loc[:len("/c-knowledge/#oidc_error=")] != "/c-knowledge/#oidc_error=" {
		t.Fatalf("Location = %q, want it to start with /c-knowledge/#oidc_error=", loc)
	}
}
