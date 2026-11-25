package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// Feishu (Lark) OAuth endpoints. They are compile-time constants rather than
// configuration so a misconfigured deployment cannot be tricked into sending
// the app secret to an arbitrary host (SSRF / credential exfiltration).
const (
	feishuTokenEndpoint    = "https://open.feishu.cn/open-apis/authen/v2/oauth/token"
	feishuUserInfoEndpoint = "https://open.feishu.cn/open-apis/authen/v1/user_info"
	// feishuDefaultEmailDomain is used when Feishu does not return an email.
	feishuDefaultEmailDomain = "dsh.c-smart.hk"
)

var (
	// ErrFeishuAuthDisabled is returned when app_id/app_secret are missing.
	ErrFeishuAuthDisabled = errors.New("feishu login is disabled")
	// ErrFeishuTenantMismatch is returned when the configured tenant_key does
	// not match the one Feishu reports for the authenticated user.
	ErrFeishuTenantMismatch = errors.New("feishu account does not belong to the configured tenant")
	// ErrFeishuIdentityConflict means the account matching the Feishu email is
	// already bound to a different open_id; we refuse to silently rebind it.
	ErrFeishuIdentityConflict = errors.New("feishu identity is already linked to another account")
)

// feishuOpenIDLookup is implemented by the GORM user repository. It is kept
// out of the broad UserRepository interface so unrelated test doubles and
// alternate repositories do not have to grow a Feishu-only method.
type feishuOpenIDLookup interface {
	GetUserByFeishuOpenID(ctx context.Context, openID string) (*types.User, error)
}

// FeishuUserInfo is the subset of /authen/v1/user_info we rely on. It is
// exported so handler tests and future integrations can construct fixtures.
type FeishuUserInfo struct {
	OpenID          string `json:"open_id"`
	UnionID         string `json:"union_id"`
	Name            string `json:"name"`
	EnName          string `json:"en_name"`
	Email           string `json:"email"`
	EnterpriseEmail string `json:"enterprise_email"`
	TenantKey       string `json:"tenant_key"`
	AvatarURL       string `json:"avatar_url"`
}

type feishuTokenResponse struct {
	Code        int    `json:"code"`
	Msg         string `json:"msg"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type feishuUserInfoEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data *FeishuUserInfo `json:"data"`
}

// getFeishuAuthConfig returns a copy of the effective Feishu configuration or
// ErrFeishuAuthDisabled. Defaults are re-applied here because tests and any
// embedded caller may construct the struct without going through LoadConfig.
func (s *userService) getFeishuAuthConfig() (*config.FeishuAuthConfig, error) {
	if s.config == nil || !s.config.FeishuAuth.IsEnabled() {
		return nil, ErrFeishuAuthDisabled
	}
	cfg := *s.config.FeishuAuth
	if cfg.SharedTenantID == 0 {
		// Single source of truth: config owns both the env override and the
		// historical 10000 fallback.
		cfg.SharedTenantID = config.DefaultSharedTenantID()
	}
	if strings.TrimSpace(cfg.DefaultRole) == "" {
		cfg.DefaultRole = string(types.TenantRoleViewer)
	}
	if !types.TenantRole(cfg.DefaultRole).IsValid() {
		return nil, fmt.Errorf("invalid feishu default_role %q", cfg.DefaultRole)
	}
	return &cfg, nil
}

// LoginWithFeishu exchanges a Feishu authorization code, resolves or creates
// the local account, guarantees membership in the shared tenant, and issues a
// token pair scoped to that tenant.
func (s *userService) LoginWithFeishu(
	ctx context.Context,
	code, redirectURI string,
) (*types.OIDCCallbackResponse, error) {
	code = strings.TrimSpace(code)
	redirectURI = strings.TrimSpace(redirectURI)
	if code == "" {
		return nil, errors.New("code is required")
	}
	if redirectURI == "" {
		return nil, errors.New("redirect_uri is required")
	}

	cfg, err := s.getFeishuAuthConfig()
	if err != nil {
		return nil, err
	}

	token, err := exchangeFeishuCode(ctx, cfg, code, redirectURI)
	if err != nil {
		return nil, err
	}
	info, err := fetchFeishuUserInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	if err := VerifyFeishuTenantKey(info, cfg.TenantKey); err != nil {
		return nil, err
	}

	user, isNewUser, err := s.findOrProvisionFeishuUser(ctx, info)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return &types.OIDCCallbackResponse{Success: false, Message: "Account is disabled"}, nil
	}

	if err := s.ensureFeishuSharedMembership(ctx, user, cfg); err != nil {
		return nil, err
	}

	// Land every Feishu session in the shared workspace. Writing the home
	// tenant (for brand-new tenantless identities) and the last-active
	// preference keeps refresh-token logins consistent: RefreshToken has no
	// tenant claim and re-resolves through resolveLoginTenantID.
	sharedTenantID := cfg.SharedTenantID
	if user.TenantID == 0 {
		user.TenantID = sharedTenantID
	}
	user.Preferences.LastActiveTenantID = &sharedTenantID
	user.UpdatedAt = time.Now()
	if err := s.userRepo.UpdateUser(ctx, user); err != nil {
		// The token below is still scoped correctly; only the next login /
		// refresh would fall back to the home tenant. Log and continue.
		logger.Warnf(ctx, "feishu login: failed to persist shared tenant preference for user %s: %v", user.ID, err)
	}

	accessToken, refreshToken, err := s.generateTokensForTenant(ctx, user, sharedTenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate local tokens: %w", err)
	}

	var tenant *types.Tenant
	if s.tenantService != nil {
		if t, terr := s.tenantService.GetTenantByID(ctx, sharedTenantID); terr == nil {
			tenant = t
		} else {
			logger.Warnf(ctx, "feishu login: failed to load shared tenant %d: %v", sharedTenantID, terr)
		}
	}
	memberships := s.buildMembershipsForUser(ctx, user, tenant)

	return &types.OIDCCallbackResponse{
		Success:      true,
		Message:      "登录成功",
		User:         user,
		Tenant:       tenant,
		Memberships:  memberships,
		Token:        accessToken,
		RefreshToken: refreshToken,
		IsNewUser:    isNewUser,
	}, nil
}

// exchangeFeishuCode performs the authorization_code exchange against the
// fixed Feishu token endpoint. Responses are capped and never echoed back:
// the error text only carries the HTTP/code status, not token material.
func exchangeFeishuCode(
	ctx context.Context,
	cfg *config.FeishuAuthConfig,
	code, redirectURI string,
) (*feishuTokenResponse, error) {
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     cfg.AppID,
		"client_secret": cfg.AppSecret,
		"code":          code,
		"redirect_uri":  redirectURI,
	})
	if err != nil {
		return nil, errors.New("failed to build feishu token request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, feishuTokenEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("failed to create feishu token request")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")

	resp, err := newFeishuHTTPClient().Do(req)
	if err != nil {
		return nil, errors.New("failed to reach feishu token endpoint")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("feishu token exchange failed: status=%d", resp.StatusCode)
	}
	var token feishuTokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil {
		return nil, errors.New("failed to decode feishu token response")
	}
	if token.Code != 0 {
		return nil, fmt.Errorf("feishu token exchange failed: code=%d", token.Code)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, errors.New("feishu token response missing access_token")
	}
	return &token, nil
}

// fetchFeishuUserInfo calls the fixed user_info endpoint with the user access
// token. The token is never logged or included in returned errors.
func fetchFeishuUserInfo(ctx context.Context, accessToken string) (*FeishuUserInfo, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("feishu access token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feishuUserInfoEndpoint, nil)
	if err != nil {
		return nil, errors.New("failed to create feishu user_info request")
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := newFeishuHTTPClient().Do(req)
	if err != nil {
		return nil, errors.New("failed to reach feishu user_info endpoint")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("feishu user_info request failed: status=%d", resp.StatusCode)
	}
	var envelope feishuUserInfoEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return nil, errors.New("failed to decode feishu user_info response")
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("feishu user_info request failed: code=%d", envelope.Code)
	}
	if envelope.Data == nil || strings.TrimSpace(envelope.Data.OpenID) == "" {
		return nil, errors.New("feishu user_info response missing open_id")
	}
	return envelope.Data, nil
}

func newFeishuHTTPClient() *http.Client {
	cfg := secutils.DefaultSSRFSafeHTTPClientConfig()
	cfg.Timeout = 15 * time.Second
	return secutils.NewSSRFSafeHTTPClient(cfg)
}

// VerifyFeishuTenantKey enforces the optional tenant_key allow-list. An empty
// configured value means "no extra check" (the Feishu app itself is already
// scoped to one tenant).
func VerifyFeishuTenantKey(info *FeishuUserInfo, configured string) error {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return nil
	}
	if info == nil || strings.TrimSpace(info.TenantKey) != configured {
		return ErrFeishuTenantMismatch
	}
	return nil
}

// findOrProvisionFeishuUser resolves the local account for a Feishu identity.
// Lookup order:
//  1. feishu_open_id — the durable identity binding, so repeat logins (even
//     after an email change) hit the same account.
//  2. email — reuse a pre-existing WeKnora account and bind the open_id to
//     it, mirroring LoginWithOIDC's email-based account linking.
//  3. create a tenantless account, then bind the open_id.
func (s *userService) findOrProvisionFeishuUser(
	ctx context.Context,
	info *FeishuUserInfo,
) (*types.User, bool, error) {
	openID := strings.TrimSpace(info.OpenID)
	if openID == "" {
		return nil, false, errors.New("feishu open_id is required")
	}

	lookup, ok := s.userRepo.(feishuOpenIDLookup)
	if !ok {
		return nil, false, errors.New("user repository does not support feishu identity lookup")
	}
	existing, err := lookup.GetUserByFeishuOpenID(ctx, openID)
	if err != nil && !isUserLookupNotFound(err) {
		return nil, false, fmt.Errorf("failed to query user by feishu open_id: %w", err)
	}
	if err == nil && existing != nil {
		return existing, false, nil
	}

	if email := feishuExplicitEmail(info); email != "" {
		existing, err = s.userRepo.GetUserByEmail(ctx, email)
		if err != nil && !isUserLookupNotFound(err) {
			return nil, false, fmt.Errorf("failed to query user by email: %w", err)
		}
		if err == nil && existing != nil {
			if existing.FeishuOpenID != nil &&
				strings.TrimSpace(*existing.FeishuOpenID) != "" &&
				*existing.FeishuOpenID != openID {
				return nil, false, ErrFeishuIdentityConflict
			}
			existing.FeishuOpenID = &openID
			existing.UpdatedAt = time.Now()
			if err := s.userRepo.UpdateUser(ctx, existing); err != nil {
				return nil, false, fmt.Errorf("failed to bind feishu identity: %w", err)
			}
			return existing, false, nil
		}
	}

	base := feishuUsernameBase(info)
	for attempt := 0; attempt < 20; attempt++ {
		username := base
		if attempt > 0 {
			username = fmt.Sprintf("%s-%d", base, attempt+1)
		}
		email := feishuAccountEmail(info, username)
		if feishuExplicitEmail(info) == "" {
			// The generated email embeds the username, so a username clash
			// also changes the email; check it before hitting Register.
			other, lookupErr := s.userRepo.GetUserByEmail(ctx, email)
			if lookupErr != nil && !isUserLookupNotFound(lookupErr) {
				return nil, false, fmt.Errorf("failed to check feishu email: %w", lookupErr)
			}
			if lookupErr == nil && other != nil {
				continue
			}
		}

		password, err := generateRandomString(32)
		if err != nil {
			return nil, false, errors.New("failed to generate password for feishu user")
		}
		user, err := s.Register(ctx, &types.RegisterRequest{
			Username:           username,
			Email:              email,
			Password:           password,
			TenantProvisioning: types.TenantProvisioningTenantless,
		})
		if err != nil {
			if errors.Is(err, ErrUserUsernameExists) || errors.Is(err, ErrUserEmailExists) {
				continue
			}
			return nil, false, fmt.Errorf("failed to auto-provision feishu user: %w", err)
		}

		user.FeishuOpenID = &openID
		feishuOnly := true
		user.Preferences.OidcOnlyLogin = &feishuOnly
		user.UpdatedAt = time.Now()
		if err := s.userRepo.UpdateUser(ctx, user); err != nil {
			return nil, false, fmt.Errorf("failed to persist feishu identity: %w", err)
		}
		return user, true, nil
	}
	return nil, false, errors.New("failed to allocate a unique username for feishu user")
}

// ensureFeishuSharedMembership makes sure the user is an active member of the
// configured shared tenant. Membership is a hard requirement: the middleware
// refuses tenant-scoped requests without it.
func (s *userService) ensureFeishuSharedMembership(
	ctx context.Context,
	user *types.User,
	cfg *config.FeishuAuthConfig,
) error {
	if s.memberService == nil {
		return errors.New("tenant membership service unavailable")
	}
	if s.tenantService != nil {
		if _, err := s.tenantService.GetTenantByID(ctx, cfg.SharedTenantID); err != nil {
			return fmt.Errorf("shared tenant %d is unavailable: %w", cfg.SharedTenantID, err)
		}
	}
	member, err := s.memberService.GetMembership(ctx, user.ID, cfg.SharedTenantID)
	if err != nil {
		return fmt.Errorf("failed to check shared tenant membership: %w", err)
	}
	if member != nil {
		if member.Status == types.TenantMemberStatusActive {
			return nil
		}
		return fmt.Errorf("shared tenant membership is not active (status=%s)", member.Status)
	}

	role := types.TenantRole(strings.TrimSpace(cfg.DefaultRole))
	if !role.IsValid() {
		role = types.TenantRoleViewer
	}
	if _, err := s.memberService.AddMember(ctx, user.ID, cfg.SharedTenantID, role, nil); err != nil {
		if errors.Is(err, ErrMembershipAlreadyExists) {
			return nil
		}
		return fmt.Errorf("failed to join shared tenant: %w", err)
	}
	return nil
}

// feishuUsernameBase derives the preferred username: email local-part first,
// then en_name, then "fs-<last 10 chars of open_id>". The result is already
// sanitised to WeKnora's username alphabet (a-z, 0-9, _, ., -).
func feishuUsernameBase(info *FeishuUserInfo) string {
	if info == nil {
		return "fs-user"
	}
	if email := feishuExplicitEmail(info); email != "" {
		if local := strings.SplitN(email, "@", 2)[0]; local != "" {
			if candidate := sanitizeUsernameCandidate(local); candidate != "" {
				return candidate
			}
		}
	}
	if candidate := sanitizeUsernameCandidate(info.EnName); candidate != "" {
		return candidate
	}
	compact := strings.ToLower(info.OpenID)
	var b strings.Builder
	for _, r := range compact {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	compact = b.String()
	if len(compact) > 10 {
		compact = compact[len(compact)-10:]
	}
	if compact == "" {
		return "fs-user"
	}
	return "fs-" + compact
}

// feishuExplicitEmail returns a usable email supplied by Feishu, lower-cased,
// or "" when neither email field carries a plausible address.
func feishuExplicitEmail(info *FeishuUserInfo) string {
	if info == nil {
		return ""
	}
	for _, raw := range []string{info.Email, info.EnterpriseEmail} {
		email := strings.ToLower(strings.TrimSpace(raw))
		if isPlausibleEmail(email) {
			return email
		}
	}
	return ""
}

// feishuAccountEmail picks the address stored on the WeKnora account: the
// Feishu-provided email when present, otherwise <username>@dsh.c-smart.hk.
func feishuAccountEmail(info *FeishuUserInfo, username string) string {
	if email := feishuExplicitEmail(info); email != "" {
		return email
	}
	return username + "@" + feishuDefaultEmailDomain
}

func isPlausibleEmail(value string) bool {
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	parts := strings.Split(value, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".")
}
