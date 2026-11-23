package types

import "testing"

func TestKnowledgeBaseVisibleTo(t *testing.T) {
	owner := Caller{TenantID: 10000, UserID: "owner-1", Role: TenantRoleContributor}
	other := Caller{TenantID: 10000, UserID: "other-1", Role: TenantRoleViewer}
	admin := Caller{TenantID: 10000, UserID: "admin-1", Role: TenantRoleAdmin}
	worker := Caller{TenantID: 10000}

	cases := []struct {
		name     string
		kb       *KnowledgeBase
		caller   Caller
		isMember bool
		want     bool
	}{
		{"public visible to everyone", &KnowledgeBase{Scope: KBScopePublic}, other, false, true},
		{"legacy empty scope behaves as public", &KnowledgeBase{}, other, false, true},
		{"personal visible to owner", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, owner, false, true},
		{"personal hidden from others", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, other, false, false},
		{"personal visible to workspace admin", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, admin, false, true},
		{"project hidden without membership", &KnowledgeBase{Scope: KBScopeProject, OwnerUserID: "owner-1"}, other, false, false},
		{"project visible to explicit member", &KnowledgeBase{Scope: KBScopeProject, OwnerUserID: "owner-1"}, other, true, true},
		{"project visible to owner", &KnowledgeBase{Scope: KBScopeProject, OwnerUserID: "owner-1"}, owner, false, true},
		{"service caller is not restricted", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, worker, false, true},
		{"nil kb is invisible", nil, owner, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.kb.VisibleTo(tc.caller, tc.isMember); got != tc.want {
				t.Fatalf("VisibleTo() = %v, want %v", got, tc.want)
			}
		})
	}
}
