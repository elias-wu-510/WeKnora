package types

import "time"

// KBMemberRole is the per-knowledge-base role granted to a workspace member.
// It is independent of the workspace-level TenantRole: a workspace Viewer can
// be a KB Contributor for one project library without gaining write access to
// the rest of the workspace.
type KBMemberRole string

const (
	KBMemberViewer      KBMemberRole = "viewer"
	KBMemberContributor KBMemberRole = "contributor"
	KBMemberAdmin       KBMemberRole = "admin"
)

// IsValid reports whether the role is one of the three supported values.
func (r KBMemberRole) IsValid() bool {
	switch r {
	case KBMemberViewer, KBMemberContributor, KBMemberAdmin:
		return true
	default:
		return false
	}
}

// CanWrite reports whether the role may add or change content in the KB.
func (r KBMemberRole) CanWrite() bool {
	return r == KBMemberContributor || r == KBMemberAdmin
}

// KBMember is an explicit per-KB grant used by project-scope visibility.
type KBMember struct {
	ID        string       `json:"id"         gorm:"type:varchar(36);primaryKey"`
	TenantID  uint64       `json:"tenant_id"  gorm:"column:tenant_id;index"`
	KBID      string       `json:"kb_id"      gorm:"column:kb_id;index"`
	UserID    string       `json:"user_id"    gorm:"column:user_id;index"`
	Role      KBMemberRole `json:"role"       gorm:"type:varchar(16);default:'viewer'"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	DeletedAt *time.Time   `json:"-"          gorm:"index"`
}

// TableName pins the storage name so migrations and GORM never diverge.
func (KBMember) TableName() string { return "kb_members" }
