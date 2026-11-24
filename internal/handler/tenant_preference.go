package handler

import "github.com/Tencent/WeKnora/internal/types"

// resolvePreferredTenantID decides which workspace /auth/me should suggest to
// a returning client. It mirrors, in one place, the order the product wants:
//
//  1. the last workspace the user actively switched into, if they are still
//     an active member of it (so a fresh login lands where they left off);
//  2. the configured shared workspace, if they are still a member of it (new
//     joiners start there instead of an empty home workspace);
//  3. the home tenant (users.tenant_id), which is always valid.
//
// memberships are the caller's active memberships (BuildLoginMemberships).
// A nil/empty list means "unknown" and is treated as no membership match, so
// we never suggest a workspace the caller cannot prove access to.
//
// This is deliberately a pure function: the tenant-selection chain has caused
// production incidents three times, and keeping the decision testable without
// a database is part of the fix.
func resolvePreferredTenantID(
	user *types.User,
	memberships []types.Membership,
	sharedTenantID uint64,
) uint64 {
	if user == nil {
		return 0
	}

	isMember := func(tenantID uint64) bool {
		if tenantID == 0 {
			return false
		}
		for _, membership := range memberships {
			if membership.TenantID == tenantID {
				return true
			}
		}
		return false
	}

	if lastActive := user.Preferences.LastActiveTenantID; lastActive != nil &&
		*lastActive != 0 && isMember(*lastActive) {
		return *lastActive
	}
	if sharedTenantID != 0 && isMember(sharedTenantID) {
		return sharedTenantID
	}
	return user.TenantID
}

// sharedTenantID returns the configured shared workspace ID, or 0 when the
// deployment has no shared workspace configured. Nil-safe because auth
// handlers are also constructed in tests and lite builds with partial config.
func (h *AuthHandler) sharedTenantID() uint64 {
	if h == nil || h.configInfo == nil || h.configInfo.FeishuAuth == nil {
		return 0
	}
	return h.configInfo.FeishuAuth.SharedTenantID
}
