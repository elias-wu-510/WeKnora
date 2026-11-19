package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

type stubFeishuUserService struct {
	interfaces.UserService
	login func(ctx context.Context, code, redirectURI string) (*types.OIDCCallbackResponse, error)
}

func (s *stubFeishuUserService) LoginWithFeishu(ctx context.Context, code, redirectURI string) (*types.OIDCCallbackResponse, error) {
	return s.login(ctx, code, redirectURI)
}

func feishuTestConfig() *config.Config {
	return &config.Config{FeishuAuth: &config.FeishuAuthConfig{
		AppID:          "cli_test_app",
		AppSecret:      "test_secret",
		SharedTenantID: 10000,
		DefaultRole:    "viewer",
	}}
}

func newFeishuTestRouter(h *AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/feishu/start", h.FeishuStart)
	r.GET("/auth/feishu/callback", h.FeishuCallback)
	r.GET("/auth/config", h.GetAuthConfig)
	return r
}

func TestFeishuStartDisabledReturns404(t *testing.T) {
	h := NewAuthHandler(&config.Config{}, &stubFeishuUserService{}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/feishu/start", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Fatalf("disabled start must not redirect, got Location=%q", loc)
	}
}

func TestFeishuStartRedirectsWithSignedState(t *testing.T) {
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/start", nil)
	req.Host = "llm-ai.c-smart.hk"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Prefix", "/c-knowledge")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body=%s, want 303", w.Code, w.Body.String())
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if location.Scheme+"://"+location.Host+location.Path != feishuAuthorizeEndpoint {
		t.Fatalf("Location = %q, want the Feishu authorize endpoint", location.String())
	}
	q := location.Query()
	if q.Get("client_id") != "cli_test_app" {
		t.Errorf("client_id = %q, want cli_test_app", q.Get("client_id"))
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want code", q.Get("response_type"))
	}
	wantRedirect := "https://llm-ai.c-smart.hk/c-knowledge/api/v1/auth/feishu/callback"
	if q.Get("redirect_uri") != wantRedirect {
		t.Errorf("redirect_uri = %q, want %q", q.Get("redirect_uri"), wantRedirect)
	}

	state, err := secutils.VerifyOIDCState(q.Get("state"))
	if err != nil {
		t.Fatalf("state did not verify: %v", err)
	}
	if state.RedirectURI != wantRedirect {
		t.Errorf("state redirect_uri = %q, want %q", state.RedirectURI, wantRedirect)
	}

	var nonce string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == feishuNonceCookieName {
			nonce = ck.Value
		}
	}
	if nonce == "" {
		t.Fatal("feishu nonce cookie was not set")
	}
	if nonce != state.Nonce {
		t.Errorf("cookie nonce %q != state nonce %q", nonce, state.Nonce)
	}
}

func TestFeishuStartPrefersConfiguredRedirectURI(t *testing.T) {
	cfg := feishuTestConfig()
	cfg.FeishuAuth.RedirectURI = "https://login.example.com/feishu/callback"
	h := NewAuthHandler(cfg, &stubFeishuUserService{}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/start", nil)
	req.Host = "internal:8080"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if got := location.Query().Get("redirect_uri"); got != cfg.FeishuAuth.RedirectURI {
		t.Fatalf("redirect_uri = %q, want configured %q", got, cfg.FeishuAuth.RedirectURI)
	}
}

func signFeishuTestState(t *testing.T, nonce, redirectURI string, issuedAt time.Time) string {
	t.Helper()
	state, err := secutils.SignOIDCState(&secutils.OIDCStatePayload{
		Nonce:       nonce,
		RedirectURI: redirectURI,
		IssuedAt:    issuedAt.Unix(),
	})
	if err != nil {
		t.Fatalf("SignOIDCState: %v", err)
	}
	return state
}

func TestFeishuCallbackRejectsMissingNonceCookie(t *testing.T) {
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{
		login: func(context.Context, string, string) (*types.OIDCCallbackResponse, error) {
			t.Error("LoginWithFeishu must not run when the browser binding is missing")
			return nil, nil
		},
	}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	state := signFeishuTestState(t, "nonce-1", "https://x/callback", time.Now())
	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/callback?code=abc&state="+url.QueryEscape(state), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "#oidc_error=invalid_state") {
		t.Fatalf("Location = %q, want invalid_state", loc)
	}
}

func TestFeishuCallbackRejectsNonceMismatch(t *testing.T) {
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{
		login: func(context.Context, string, string) (*types.OIDCCallbackResponse, error) {
			t.Error("LoginWithFeishu must not run on nonce mismatch")
			return nil, nil
		},
	}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	state := signFeishuTestState(t, "nonce-1", "https://x/callback", time.Now())
	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/callback?code=abc&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: feishuNonceCookieName, Value: "different"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); !strings.Contains(loc, "#oidc_error=invalid_state") {
		t.Fatalf("Location = %q, want invalid_state", loc)
	}
}

func TestFeishuCallbackRejectsExpiredState(t *testing.T) {
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{
		login: func(context.Context, string, string) (*types.OIDCCallbackResponse, error) {
			t.Error("LoginWithFeishu must not run on expired state")
			return nil, nil
		},
	}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	state := signFeishuTestState(t, "nonce-1", "https://x/callback", time.Now().Add(-11*time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/callback?code=abc&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: feishuNonceCookieName, Value: "nonce-1"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); !strings.Contains(loc, "#oidc_error=invalid_state") {
		t.Fatalf("Location = %q, want invalid_state", loc)
	}
}

func TestFeishuCallbackSuccessRedirectsWithOIDCPayload(t *testing.T) {
	var gotCode, gotRedirect string
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{
		login: func(_ context.Context, code, redirectURI string) (*types.OIDCCallbackResponse, error) {
			gotCode, gotRedirect = code, redirectURI
			return &types.OIDCCallbackResponse{Success: true, Token: "jwt-token", RefreshToken: "refresh-token"}, nil
		},
	}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	callbackURL := "https://llm-ai.c-smart.hk/c-knowledge/api/v1/auth/feishu/callback"
	state := signFeishuTestState(t, "nonce-1", callbackURL, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/callback?code=the-code&state="+url.QueryEscape(state), nil)
	req.Host = "llm-ai.c-smart.hk"
	req.Header.Set("X-Forwarded-Prefix", "/c-knowledge")
	req.AddCookie(&http.Cookie{Name: feishuNonceCookieName, Value: "nonce-1"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if gotCode != "the-code" || gotRedirect != callbackURL {
		t.Fatalf("service got (code=%q, redirect=%q), want (the-code, %s)", gotCode, gotRedirect, callbackURL)
	}
	loc := w.Header().Get("Location")
	const prefix = "/c-knowledge/#oidc_result="
	if !strings.HasPrefix(loc, prefix) {
		t.Fatalf("Location = %q, want prefix %q", loc, prefix)
	}
	payload, err := url.QueryUnescape(strings.TrimPrefix(loc, prefix))
	if err != nil {
		t.Fatalf("unescape payload: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded["token"] != "jwt-token" {
		t.Fatalf("payload token = %v, want jwt-token", decoded["token"])
	}

	var cleared bool
	for _, ck := range w.Result().Cookies() {
		if ck.Name == feishuNonceCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("feishu nonce cookie should be cleared after use")
	}
}

func TestFeishuCallbackTenantMismatchIsSanitised(t *testing.T) {
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{
		login: func(context.Context, string, string) (*types.OIDCCallbackResponse, error) {
			return nil, service.ErrFeishuTenantMismatch
		},
	}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	state := signFeishuTestState(t, "nonce-1", "https://x/callback", time.Now())
	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/callback?code=abc&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: feishuNonceCookieName, Value: "nonce-1"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "#oidc_error=login_failed") {
		t.Fatalf("Location = %q, want login_failed", loc)
	}
	if !strings.Contains(loc, "does+not+belong") && !strings.Contains(loc, "does%20not%20belong") {
		t.Fatalf("Location = %q, want the tenant mismatch description", loc)
	}
}

func TestGetAuthConfigExposesFeishuEnabled(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		cfg := &config.Config{}
		if enabled {
			cfg.FeishuAuth = &config.FeishuAuthConfig{AppID: "cli", AppSecret: "secret"}
		}
		h := &AuthHandler{configInfo: cfg}
		r := newFeishuTestRouter(h)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/config", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		got, ok := body["feishu_enabled"].(bool)
		if !ok {
			t.Fatalf("feishu_enabled missing or not bool: %v", body["feishu_enabled"])
		}
		if got != enabled {
			t.Fatalf("feishu_enabled = %v, want %v", got, enabled)
		}
	}
}

func TestFeishuCallbackRejectsTamperedState(t *testing.T) {
	h := NewAuthHandler(feishuTestConfig(), &stubFeishuUserService{
		login: func(context.Context, string, string) (*types.OIDCCallbackResponse, error) {
			t.Error("LoginWithFeishu must not run on a tampered state")
			return nil, nil
		},
	}, nil, nil, nil)
	r := newFeishuTestRouter(h)

	state := signFeishuTestState(t, "nonce-1", "https://x/callback", time.Now())
	state = state[:len(state)-2] + "aa"
	req := httptest.NewRequest(http.MethodGet, "/auth/feishu/callback?code=abc&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: feishuNonceCookieName, Value: "nonce-1"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); !strings.Contains(loc, "#oidc_error=invalid_state") {
		t.Fatalf("Location = %q, want invalid_state", loc)
	}
}
