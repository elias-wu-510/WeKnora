package handler

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// TestBuildKBResponse_ExtrasKeyContract pins the exact set of JSON keys the
// typed kbResponseExtras DTO may add (or strip) for the three audiences the
// KB detail page serves: a same-tenant KB with no extras, a cross-tenant
// shared KB (my_permission), and an admin (can_manage_members + capabilities
// folded into the existing feature-flag object).
//
// Keys are compared as a set via map[string]json.RawMessage so a field rename
// or an accidental extra key fails even when the value would be ignored by
// older clients.
func TestBuildKBResponse_ExtrasKeyContract(t *testing.T) {
	storeID := "store-uuid"
	kb := &types.KnowledgeBase{
		ID:               "kb-1",
		Name:             "contract-kb",
		Type:             types.KnowledgeBaseTypeDocument,
		TenantID:         1,
		CreatorID:        "u-owner",
		EmbeddingModelID: "embedding",
		SummaryModelID:   "summary",
		VectorStoreID:    &storeID,
	}
	base := kbRawKeySet(t, kb)

	canManage := true
	cannotManage := false

	for _, tt := range []struct {
		name    string
		view    types.StoreDisplay
		extras  kbResponseExtras
		addKeys []string
		delKeys []string
	}{
		{
			name:    "same-tenant KB without extras",
			view:    types.DefaultStoreDisplay(),
			extras:  kbResponseExtras{},
			addKeys: []string{"vector_store_name", "vector_store_source", "vector_store_status"},
		},
		{
			name:    "cross-tenant shared KB carries my_permission",
			view:    types.SharedStoreDisplay(),
			extras:  kbResponseExtras{MyPermission: string(types.OrgRoleViewer)},
			addKeys: []string{"vector_store_source", "vector_store_status", "my_permission"},
			delKeys: []string{"vector_store_id"},
		},
		{
			name: "admin viewing KB carries can_manage_members true",
			view: types.DefaultStoreDisplay(),
			extras: kbResponseExtras{
				CanManageMembers: &canManage,
				Capabilities: map[string]bool{
					"read": true, "upload": true, "edit": true,
					"delete": true, "manage_members": true, "manage_kb": true,
				},
			},
			addKeys: []string{
				"vector_store_name", "vector_store_source", "vector_store_status",
				"can_manage_members",
			},
		},
		{
			// The detail handler always sets the flag, so false must stay on
			// the wire; dropping it with omitempty would change the contract.
			name:    "non-admin can_manage_members false stays present",
			view:    types.DefaultStoreDisplay(),
			extras:  kbResponseExtras{CanManageMembers: &cannotManage},
			addKeys: []string{"vector_store_name", "vector_store_source", "vector_store_status", "can_manage_members"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := buildKBResponse(kb, tt.view, tt.extras)
			require.NotNil(t, got)

			want := make(map[string]struct{}, len(base)+len(tt.addKeys))
			for key := range base {
				want[key] = struct{}{}
			}
			for _, key := range tt.delKeys {
				delete(want, key)
			}
			for _, key := range tt.addKeys {
				want[key] = struct{}{}
			}

			gotRaw, err := json.Marshal(got)
			require.NoError(t, err)
			var gotKeys map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(gotRaw, &gotKeys))

			require.Equal(t, sortedKeys(want), sortedKeys(gotKeys), "unexpected JSON key set: %s", gotRaw)
		})
	}

	// Value-level spot checks: the keys must carry the same scalar values the
	// old map extras produced.
	shared := buildKBResponse(kb, types.SharedStoreDisplay(), kbResponseExtras{
		MyPermission: string(types.OrgRoleEditor),
	})
	var permission string
	require.NoError(t, json.Unmarshal(shared["my_permission"], &permission))
	require.Equal(t, string(types.OrgRoleEditor), permission)

	admin := buildKBResponse(kb, types.DefaultStoreDisplay(), kbResponseExtras{
		CanManageMembers: &canManage,
		Capabilities:     map[string]bool{"read": true, "manage_kb": true},
	})
	var managed bool
	require.NoError(t, json.Unmarshal(admin["can_manage_members"], &managed))
	require.True(t, managed)
	var caps map[string]bool
	require.NoError(t, json.Unmarshal(admin["capabilities"], &caps))
	require.True(t, caps["read"])
	require.True(t, caps["manage_kb"])
	require.Contains(t, caps, "vector", "feature flags must survive the extras merge")
}

// kbRawKeySet returns the keys KnowledgeBase.MarshalJSON emits, which is the
// baseline buildKBResponse must preserve verbatim.
func kbRawKeySet(t *testing.T, kb *types.KnowledgeBase) map[string]struct{} {
	t.Helper()
	raw, err := json.Marshal(kb)
	require.NoError(t, err)
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	out := make(map[string]struct{}, len(decoded))
	for key := range decoded {
		out[key] = struct{}{}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
