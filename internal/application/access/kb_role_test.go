package access

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestResolveEffectiveKBRole(t *testing.T) {
	for _, tt := range []struct {
		name    string
		sources KBRoleSources
		want    KBRole
	}{
		{name: "no source is none", want: KBRoleNone},
		{
			name:    "service principal keeps machine admin",
			sources: KBRoleSources{IsServicePrincipal: true},
			want:    KBRoleAdmin,
		},
		{
			name:    "workspace owner is admin",
			sources: KBRoleSources{TenantRole: types.TenantRoleOwner},
			want:    KBRoleAdmin,
		},
		{
			name:    "workspace admin is admin",
			sources: KBRoleSources{TenantRole: types.TenantRoleAdmin},
			want:    KBRoleAdmin,
		},
		{
			name:    "workspace contributor is contributor",
			sources: KBRoleSources{TenantRole: types.TenantRoleContributor},
			want:    KBRoleContributor,
		},
		{
			name:    "workspace viewer is viewer",
			sources: KBRoleSources{TenantRole: types.TenantRoleViewer},
			want:    KBRoleViewer,
		},
		{
			name: "workspace viewer plus kb contributor is contributor",
			sources: KBRoleSources{
				TenantRole:   types.TenantRoleViewer,
				KBMemberRole: types.KBMemberContributor,
			},
			want: KBRoleContributor,
		},
		{
			name: "workspace contributor plus kb viewer stays contributor",
			sources: KBRoleSources{
				TenantRole:   types.TenantRoleContributor,
				KBMemberRole: types.KBMemberViewer,
			},
			want: KBRoleContributor,
		},
		{
			name: "workspace viewer plus kb admin is admin",
			sources: KBRoleSources{
				TenantRole:   types.TenantRoleViewer,
				KBMemberRole: types.KBMemberAdmin,
			},
			want: KBRoleAdmin,
		},
		{
			name: "org share editor maps to contributor",
			sources: KBRoleSources{
				TenantRole: types.TenantRoleViewer,
				ShareRole:  types.OrgRoleEditor,
			},
			want: KBRoleContributor,
		},
		{
			name: "org share admin maps to admin",
			sources: KBRoleSources{
				TenantRole: types.TenantRoleViewer,
				ShareRole:  types.OrgRoleAdmin,
			},
			want: KBRoleAdmin,
		},
		{
			name: "kb owner is admin even with viewer role",
			sources: KBRoleSources{
				TenantRole: types.TenantRoleViewer,
				IsKBOwner:  true,
			},
			want: KBRoleAdmin,
		},
		{
			name: "highest source wins",
			sources: KBRoleSources{
				TenantRole:   types.TenantRoleAdmin,
				KBMemberRole: types.KBMemberViewer,
				ShareRole:    types.OrgRoleViewer,
			},
			want: KBRoleAdmin,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ResolveEffectiveKBRole(tt.sources))
		})
	}
}

func TestKBRoleOrgMemberRoleProjection(t *testing.T) {
	require.Equal(t, types.OrgMemberRole(""), KBRoleNone.OrgMemberRole())
	require.Equal(t, types.OrgRoleViewer, KBRoleViewer.OrgMemberRole())
	require.Equal(t, types.OrgRoleEditor, KBRoleContributor.OrgMemberRole())
	require.Equal(t, types.OrgRoleAdmin, KBRoleAdmin.OrgMemberRole())
	require.True(t, KBRoleAdmin.HasPermission(KBRoleContributor))
	require.False(t, KBRoleViewer.HasPermission(KBRoleContributor))
}

func TestResolveKBEffectiveRoleMatrix(t *testing.T) {
	for _, tt := range []struct {
		name         string
		kbTenant     uint64
		callerTenant uint64
		callerUserID string
		callerRole   types.TenantRole
		kbMemberRole types.KBMemberRole
		creatorID    string
		required     types.OrgMemberRole
		want         types.OrgMemberRole
		wantErr      error
	}{
		{
			name:         "workspace viewer with kb contributor may write",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			kbMemberRole: types.KBMemberContributor,
			required:     types.OrgRoleEditor,
			want:         types.OrgRoleEditor,
		},
		{
			name:         "workspace viewer with kb viewer may not write",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			kbMemberRole: types.KBMemberViewer,
			required:     types.OrgRoleEditor,
			wantErr:      ErrForbidden,
		},
		{
			name:         "workspace viewer with kb admin may delete",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			kbMemberRole: types.KBMemberAdmin,
			required:     types.OrgRoleAdmin,
			want:         types.OrgRoleAdmin,
		},
		{
			name:         "workspace viewer with kb contributor may not delete",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			kbMemberRole: types.KBMemberContributor,
			required:     types.OrgRoleAdmin,
			wantErr:      ErrForbidden,
		},
		{
			name:         "workspace admin is allowed everything",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleAdmin,
			required:     types.OrgRoleAdmin,
			want:         types.OrgRoleAdmin,
		},
		{
			name:         "workspace viewer reads without kb role",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			required:     types.OrgRoleViewer,
			want:         types.OrgRoleViewer,
		},
		{
			name:         "kb creator keeps admin projection",
			kbTenant:     1,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			creatorID:    "member",
			required:     types.OrgRoleAdmin,
			want:         types.OrgRoleAdmin,
		},
		{
			name:         "cross tenant kb member does not bypass org share",
			kbTenant:     2,
			callerTenant: 1,
			callerUserID: "member",
			callerRole:   types.TenantRoleViewer,
			kbMemberRole: types.KBMemberContributor,
			required:     types.OrgRoleEditor,
			wantErr:      ErrForbidden,
		},
		{
			name:         "non member non share is forbidden",
			kbTenant:     2,
			callerTenant: 1,
			callerUserID: "outsider",
			callerRole:   types.TenantRoleViewer,
			required:     types.OrgRoleViewer,
			wantErr:      ErrForbidden,
		},
		{
			name:         "service principal keeps admin projection",
			kbTenant:     1,
			callerTenant: 1,
			callerRole:   types.TenantRoleViewer,
			required:     types.OrgRoleAdmin,
			want:         types.OrgRoleAdmin,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kb := &types.KnowledgeBase{ID: "kb", TenantID: tt.kbTenant, CreatorID: tt.creatorID}
			request := KBRequest{
				Caller: types.Caller{
					TenantID: tt.callerTenant,
					UserID:   tt.callerUserID,
					Role:     tt.callerRole,
				},
				KBMemberRole: tt.kbMemberRole,
			}
			grant, err := ResolveKB(context.Background(), request, kb, tt.required, nil, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, grant)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, grant.Permission)
		})
	}
}
