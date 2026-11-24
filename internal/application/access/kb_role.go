package access

import (
	"github.com/Tencent/WeKnora/internal/types"
)

// KBRole is the effective role a caller holds on one knowledge base. It folds
// the workspace role, the explicit kb_members grant, an organization share and
// KB ownership into a single ladder. The workspace role remains the baseline;
// every additional source can only raise the result, never lower it.
type KBRole string

const (
	// KBRoleNone means the caller has no access to the KB through any source.
	KBRoleNone KBRole = ""
	// KBRoleViewer may read and search the KB.
	KBRoleViewer KBRole = "viewer"
	// KBRoleContributor may add and change content in the KB.
	KBRoleContributor KBRole = "contributor"
	// KBRoleAdmin may also manage members and delete the KB.
	KBRoleAdmin KBRole = "admin"
)

// Level maps the role onto the viewer < contributor < admin ladder.
func (r KBRole) Level() int {
	switch r {
	case KBRoleViewer:
		return 1
	case KBRoleContributor:
		return 2
	case KBRoleAdmin:
		return 3
	default:
		return 0
	}
}

// IsValid reports whether the role is one of the three supported values.
func (r KBRole) IsValid() bool {
	switch r {
	case KBRoleViewer, KBRoleContributor, KBRoleAdmin:
		return true
	default:
		return false
	}
}

// HasPermission checks if this role is at least the required level.
func (r KBRole) HasPermission(required KBRole) bool {
	return r.Level() >= required.Level()
}

// OrgMemberRole converts the effective KB role to the permission vocabulary
// used by the KBAccess grant and the existing org-share checks.
func (r KBRole) OrgMemberRole() types.OrgMemberRole {
	switch r {
	case KBRoleAdmin:
		return types.OrgRoleAdmin
	case KBRoleContributor:
		return types.OrgRoleEditor
	case KBRoleViewer:
		return types.OrgRoleViewer
	default:
		return ""
	}
}

// KBRoleSources are the independent inputs to the effective-role calculation.
// ShareRole is only set for a KB reached through an organization share, and
// IsKBOwner is true for the KB creator/owner.
type KBRoleSources struct {
	TenantRole         types.TenantRole
	KBMemberRole       types.KBMemberRole
	ShareRole          types.OrgMemberRole
	IsKBOwner          bool
	IsServicePrincipal bool
}

// ResolveEffectiveKBRole returns the highest role granted by any source:
//
//	max(workspace role, kb_members.role, organization share role, KB owner)
//
// A machine principal (API key or internal service caller) is authorized by
// its own gate and keeps the historical admin-level access to the resource.
func ResolveEffectiveKBRole(sources KBRoleSources) KBRole {
	if sources.IsServicePrincipal {
		return KBRoleAdmin
	}
	role := KBRoleNone
	for _, candidate := range []KBRole{
		kbRoleFromTenantRole(sources.TenantRole),
		kbRoleFromKBMemberRole(sources.KBMemberRole),
		kbRoleFromOrgRole(sources.ShareRole),
	} {
		if candidate.HasPermission(role) {
			role = candidate
		}
	}
	if sources.IsKBOwner {
		role = KBRoleAdmin
	}
	return role
}

func kbRoleFromTenantRole(role types.TenantRole) KBRole {
	switch role {
	case types.TenantRoleOwner, types.TenantRoleAdmin:
		return KBRoleAdmin
	case types.TenantRoleContributor:
		return KBRoleContributor
	case types.TenantRoleViewer:
		return KBRoleViewer
	default:
		return KBRoleNone
	}
}

func kbRoleFromKBMemberRole(role types.KBMemberRole) KBRole {
	switch role {
	case types.KBMemberAdmin:
		return KBRoleAdmin
	case types.KBMemberContributor:
		return KBRoleContributor
	case types.KBMemberViewer:
		return KBRoleViewer
	default:
		return KBRoleNone
	}
}

func kbRoleFromOrgRole(role types.OrgMemberRole) KBRole {
	switch role {
	case types.OrgRoleAdmin:
		return KBRoleAdmin
	case types.OrgRoleEditor:
		return KBRoleContributor
	case types.OrgRoleViewer:
		return KBRoleViewer
	default:
		return KBRoleNone
	}
}
