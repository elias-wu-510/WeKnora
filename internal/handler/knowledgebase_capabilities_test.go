package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestKBCapabilitiesContract pins the server-computed capability contract
// consumed by the knowledge-base detail page. The old UI inferred these
// booleans from the workspace role plus org share; kb_members was invisible,
// so a workspace viewer with an explicit kb_members grant never saw the
// controls the backend would actually authorize.
func TestKBCapabilitiesContract(t *testing.T) {
	for _, tt := range []struct {
		name       string
		tenantRole types.TenantRole
		kbRole     types.KBMemberRole
		creatorID  string
		permission types.OrgMemberRole
		want       map[string]bool
	}{
		{
			name:       "workspace viewer with kb contributor can upload and edit",
			tenantRole: types.TenantRoleViewer,
			kbRole:     types.KBMemberContributor,
			permission: types.OrgRoleEditor,
			want: map[string]bool{
				"read": true, "upload": true, "edit": true, "delete": false, "manage_members": false,
			},
		},
		{
			name:       "workspace viewer with kb viewer is read only",
			tenantRole: types.TenantRoleViewer,
			kbRole:     types.KBMemberViewer,
			permission: types.OrgRoleViewer,
			want: map[string]bool{
				"read": true, "upload": false, "edit": false, "delete": false, "manage_members": false,
			},
		},
		{
			name:       "workspace viewer with kb admin can delete and manage members",
			tenantRole: types.TenantRoleViewer,
			kbRole:     types.KBMemberAdmin,
			permission: types.OrgRoleAdmin,
			want: map[string]bool{
				"read": true, "upload": true, "edit": true, "delete": true, "manage_members": true,
			},
		},
		{
			name:       "workspace admin can do everything",
			tenantRole: types.TenantRoleAdmin,
			permission: types.OrgRoleAdmin,
			want: map[string]bool{
				"read": true, "upload": true, "edit": true, "delete": true, "manage_members": true,
			},
		},
		{
			name:       "kb creator keeps delete and member management",
			tenantRole: types.TenantRoleViewer,
			creatorID:  "member",
			permission: types.OrgRoleAdmin,
			want: map[string]bool{
				"read": true, "upload": true, "edit": true, "delete": true, "manage_members": true,
			},
		},
		{
			name:       "plain workspace viewer cannot write",
			tenantRole: types.TenantRoleViewer,
			permission: types.OrgRoleViewer,
			want: map[string]bool{
				"read": true, "upload": false, "edit": false, "delete": false, "manage_members": false,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubKBService{role: tt.kbRole}
			h := &KnowledgeBaseHandler{service: svc}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			ctx := types.WithCaller(req.Context(), types.Caller{
				TenantID: 1,
				UserID:   "member",
				Role:     tt.tenantRole,
			})
			c.Request = req.WithContext(ctx)
			kb := &types.KnowledgeBase{ID: "kb", TenantID: 1, CreatorID: tt.creatorID, Type: types.KnowledgeBaseTypeDocument}
			require.Equal(t, tt.want, h.kbCapabilities(c, kb, tt.permission))
		})
	}
}

// guard against a future MarshalJSON/buildKBResponse change dropping the
// auth keys while merging the pre-existing feature capabilities map.
func TestKBCapabilitiesMergeKeepsFeatureFlags(t *testing.T) {
	kb := &types.KnowledgeBase{ID: "kb", TenantID: 1, Type: types.KnowledgeBaseTypeDocument}
	merged := buildKBResponse(kb, types.StoreDisplay{}, map[string]interface{}{
		"capabilities": map[string]bool{"read": true, "upload": false, "edit": false, "delete": false, "manage_members": false},
	})
	m, ok := merged.(map[string]interface{})
	require.True(t, ok)
	caps, ok := m["capabilities"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, caps["read"], "auth keys must survive the merge")
	require.Contains(t, caps, "vector", "feature flags must survive the merge")
	require.Contains(t, caps, "keyword")
	require.Contains(t, caps, "wiki")
	require.Contains(t, caps, "graph")
	require.Contains(t, caps, "faq")
}
