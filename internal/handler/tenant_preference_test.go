package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type tenantPreferenceUserService struct {
	interfaces.UserService
	user        *types.User
	memberships []types.Membership
}

func (s *tenantPreferenceUserService) GetCurrentUser(context.Context) (*types.User, error) {
	return s.user, nil
}

func (s *tenantPreferenceUserService) BuildLoginMemberships(
	context.Context, *types.User, *types.Tenant,
) []types.Membership {
	return s.memberships
}

type tenantPreferenceTenantService struct {
	interfaces.TenantService
}

func (s *tenantPreferenceTenantService) GetTenantByID(
	_ context.Context, id uint64,
) (*types.Tenant, error) {
	return &types.Tenant{ID: id, Name: "workspace"}, nil
}

func uint64Ptr(v uint64) *uint64 { return &v }

func TestResolvePreferredTenantID(t *testing.T) {
	memberships := []types.Membership{
		{TenantID: 1, TenantName: "home"},
		{TenantID: 2, TenantName: "last-active"},
		{TenantID: 10000, TenantName: "shared"},
	}
	user := func(home uint64, lastActive *uint64) *types.User {
		return &types.User{
			ID:          "u1",
			TenantID:    home,
			Preferences: types.UserPreferences{LastActiveTenantID: lastActive},
		}
	}

	cases := []struct {
		name        string
		user        *types.User
		memberships []types.Membership
		shared      uint64
		want        uint64
	}{
		{"last active wins when still a member", user(1, uint64Ptr(2)), memberships, 10000, 2},
		{"last active ignored after membership loss", user(1, uint64Ptr(2)), memberships[:1], 10000, 1},
		{"shared used when no last-active preference", user(1, nil), memberships, 10000, 10000},
		{"shared ignored when not a member", user(1, nil), memberships[:1], 10000, 1},
		{"shared ignored when unset", user(1, nil), memberships, 0, 1},
		{"zero last-active sentinel falls through", user(1, uint64Ptr(0)), memberships, 10000, 10000},
		{"unknown memberships never suggest shared", user(1, nil), nil, 10000, 1},
		{"nil user", nil, memberships, 10000, 0},
		{"home fallback with no memberships", user(7, nil), nil, 0, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want,
				resolvePreferredTenantID(tc.user, tc.memberships, tc.shared))
		})
	}
}

func TestGetCurrentUserExposesServerResolvedPreferredTenant(t *testing.T) {
	users := &tenantPreferenceUserService{
		user: &types.User{
			ID:          "u1",
			TenantID:    1,
			Preferences: types.UserPreferences{LastActiveTenantID: uint64Ptr(2)},
		},
		memberships: []types.Membership{
			{TenantID: 1, TenantName: "home"},
			{TenantID: 2, TenantName: "last-active"},
			{TenantID: 10000, TenantName: "shared"},
		},
	}
	h := &AuthHandler{
		userService:   users,
		tenantService: &tenantPreferenceTenantService{},
		configInfo: &config.Config{
			Tenant:     &config.TenantConfig{},
			FeishuAuth: &config.FeishuAuthConfig{SharedTenantID: 10000},
		},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/me", nil)

	h.GetCurrentUser(c)

	require.Equal(t, 200, w.Code)
	var payload struct {
		Data struct {
			PreferredTenantID uint64 `json:"preferred_tenant_id"`
			Memberships       []struct {
				TenantID uint64 `json:"tenant_id"`
			} `json:"memberships"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, uint64(2), payload.Data.PreferredTenantID)
	require.Len(t, payload.Data.Memberships, 3)
}

func TestGetAuthConfigExposesSharedTenantID(t *testing.T) {
	h := &AuthHandler{configInfo: &config.Config{
		FeishuAuth: &config.FeishuAuthConfig{SharedTenantID: 10000},
	}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/config", nil)

	h.GetAuthConfig(c)

	require.Equal(t, 200, w.Code)
	var payload struct {
		SharedTenantID uint64 `json:"shared_tenant_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Equal(t, uint64(10000), payload.SharedTenantID)
}
