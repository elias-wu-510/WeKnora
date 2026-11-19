package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

const (
	feishuNonceCookieName   = "weknora_feishu_nonce"
	feishuNonceCookieMaxAge = 600
	feishuAuthorizeEndpoint = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"
)

// feishuEnabled reports whether the deployment has usable Feishu app
// credentials. It is also exposed to the SPA as auth config
// "feishu_enabled" so the login page can hide the entry point.
func (h *AuthHandler) feishuEnabled() bool {
	return h != nil && h.configInfo != nil && h.configInfo.FeishuAuth.IsEnabled()
}

// feishuCallbackURL derives the absolute callback URL from the request's own
// origin and the reverse-proxy prefix (C-Knowledge at /c-knowledge).
func feishuCallbackURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + webBasePath(c) + "/api/v1/auth/feishu/callback"
}

// feishuRedirectURI prefers the explicitly configured redirect_uri (which must
// exactly match the Feishu console allow-list) over the derived one.
func feishuRedirectURI(c *gin.Context, configured string) string {
	if value := strings.TrimSpace(configured); value != "" {
		return value
	}
	return feishuCallbackURL(c)
}

func setFeishuNonceCookie(c *gin.Context, nonce string) {
	if nonce == "" {
		return
	}
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(feishuNonceCookieName, nonce, feishuNonceCookieMaxAge, "/", "", secure, true)
}

func clearFeishuNonceCookie(c *gin.Context) {
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(feishuNonceCookieName, "", -1, "/", "", secure, true)
}

type feishuStatePayload struct {
	Nonce       string
	RedirectURI string
}

// decodeFeishuState verifies the HMAC/freshness of the state and binds it to
// this browser via the nonce cookie, mirroring the OIDC flow.
func decodeFeishuState(c *gin.Context) (*feishuStatePayload, error) {
	verified, err := secutils.VerifyOIDCState(strings.TrimSpace(c.Query("state")))
	if err != nil {
		return nil, err
	}
	cookie, err := c.Request.Cookie(feishuNonceCookieName)
	if err != nil || cookie == nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, errors.New("feishu nonce cookie missing")
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(verified.Nonce)) != 1 {
		return nil, errors.New("feishu nonce mismatch")
	}
	return &feishuStatePayload{
		Nonce:       verified.Nonce,
		RedirectURI: strings.TrimSpace(verified.RedirectURI),
	}, nil
}

// FeishuStart godoc
// @Summary      发起飞书登录（直接 303）
// @Description  生成一次性签名 state 与浏览器绑定 cookie，随后 303 跳转到飞书授权页。
// @Description  app_id/app_secret 未配置时返回 404，前端据此隐藏入口。
// @Tags         认证
// @Success      303
// @Failure      404
// @Router       /auth/feishu/start [get]
func (h *AuthHandler) FeishuStart(c *gin.Context) {
	ctx := c.Request.Context()
	if !h.feishuEnabled() {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "feishu login is not configured",
		})
		return
	}
	cfg := h.configInfo.FeishuAuth
	redirectURI := feishuRedirectURI(c, cfg.RedirectURI)

	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		logger.Errorf(ctx, "Failed to generate feishu login nonce: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to start feishu login",
		})
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)

	state, err := secutils.SignOIDCState(&secutils.OIDCStatePayload{
		Nonce:       nonce,
		RedirectURI: redirectURI,
		IssuedAt:    time.Now().Unix(),
	})
	if err != nil {
		logger.Errorf(ctx, "Failed to sign feishu login state: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to start feishu login",
		})
		return
	}

	setFeishuNonceCookie(c, nonce)

	authorizeURL, err := url.Parse(feishuAuthorizeEndpoint)
	if err != nil {
		logger.Errorf(ctx, "Invalid feishu authorize endpoint: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to start feishu login",
		})
		return
	}
	query := authorizeURL.Query()
	query.Set("client_id", cfg.AppID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("state", state)
	authorizeURL.RawQuery = query.Encode()

	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusSeeOther, authorizeURL.String())
}

// FeishuCallback godoc
// @Summary      飞书登录重定向回调
// @Description  校验 state 与 nonce cookie 后由后端完成 code 交换、用户建号/绑定、
// @Description  共享租户成员关系（默认 10000）并签发 JWT，最后 303 回前端登录页。
// @Tags         认证
// @Param        code   query string false "飞书授权码"
// @Param        state  query string false "飞书状态"
// @Param        error  query string false "飞书错误码"
// @Success      303
// @Failure      404
// @Router       /auth/feishu/callback [get]
func (h *AuthHandler) FeishuCallback(c *gin.Context) {
	ctx := c.Request.Context()
	frontendRedirectURI := webBasePath(c) + "/"

	if !h.feishuEnabled() {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "feishu login is not configured",
		})
		return
	}

	if providerError := strings.TrimSpace(c.Query("error")); providerError != "" {
		clearFeishuNonceCookie(c)
		redirectURL := frontendRedirectURI + "#oidc_error=" + urlQueryEscape(providerError)
		if description := strings.TrimSpace(c.Query("error_description")); description != "" {
			redirectURL += "&oidc_error_description=" + urlQueryEscape(description)
		}
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	decodedState, err := decodeFeishuState(c)
	if err != nil {
		logger.Errorf(ctx, "Failed to decode feishu login state: %v", err)
		clearFeishuNonceCookie(c)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("invalid_state"))
		return
	}
	// One-time use: the binding cookie is cleared as soon as it is checked.
	clearFeishuNonceCookie(c)

	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("missing_code"))
		return
	}

	resp, err := h.userService.LoginWithFeishu(ctx, code, decodedState.RedirectURI)
	if err != nil {
		// Full error goes to the server log only; the browser receives a
		// sanitised code so provider/DB details are never reflected.
		logger.Errorf(ctx, "Failed to complete feishu login: %v", err)
		description := "unable to complete feishu login"
		if errors.Is(err, service.ErrFeishuTenantMismatch) {
			description = "feishu account does not belong to the configured tenant"
		}
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("login_failed")+
			"&oidc_error_description="+urlQueryEscape(description))
		return
	}
	if !resp.Success {
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("login_failed")+
			"&oidc_error_description="+urlQueryEscape(resp.Message))
		return
	}

	payload, err := encodeOIDCCallbackPayload(resp)
	if err != nil {
		logger.Errorf(ctx, "Failed to encode feishu callback payload: %v", err)
		c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_error="+urlQueryEscape("payload_encode_failed"))
		return
	}

	c.Redirect(http.StatusFound, frontendRedirectURI+"#oidc_result="+urlQueryEscape(payload))
}
