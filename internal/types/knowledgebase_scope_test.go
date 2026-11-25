package types

import (
	"encoding/json"
	"testing"
)

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
		{"tenant visible to everyone", &KnowledgeBase{Scope: KBScopeTenant}, other, false, true},
		{"legacy empty scope behaves as tenant", &KnowledgeBase{}, other, false, true},
		{"personal visible to owner", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, owner, false, true},
		{"personal hidden from others", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, other, false, false},
		{"personal visible to workspace admin", &KnowledgeBase{Scope: KBScopePersonal, OwnerUserID: "owner-1"}, admin, false, true},
		{"legacy public value still behaves as tenant", &KnowledgeBase{Scope: "public"}, other, false, true},
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

func TestNormalizeScopeInput(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", KBScopeTenant, true},
		{"tenant", KBScopeTenant, true},
		{"public", KBScopeTenant, true},   // legacy, any case below
		{"PUBLIC", KBScopeTenant, true},   // case-insensitive legacy spelling
		{" Tenant ", KBScopeTenant, true}, // surrounding whitespace tolerated
		{"project", KBScopeProject, true},
		{"Project", KBScopeProject, true},
		{"personal", KBScopePersonal, true},
		{"shared", "", false},
		{"everyone", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := NormalizeScopeInput(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizeScopeInput(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNormalizedScopeCanonicalizesLegacyValues(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		want  string
	}{
		{"empty", "", KBScopeTenant},
		{"tenant", KBScopeTenant, KBScopeTenant},
		{"legacy public", "public", KBScopeTenant},
		{"legacy public case-insensitive", "Public", KBScopeTenant},
		{"unknown", "shared", KBScopeTenant},
		{"project", KBScopeProject, KBScopeProject},
		{"personal", KBScopePersonal, KBScopePersonal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kb := &KnowledgeBase{Scope: tc.scope}
			if got := kb.NormalizedScope(); got != tc.want {
				t.Fatalf("NormalizedScope() = %q, want %q", got, tc.want)
			}
		})
	}
	var nilKB *KnowledgeBase
	if got := nilKB.NormalizedScope(); got != KBScopeTenant {
		t.Fatalf("nil NormalizedScope() = %q, want %q", got, KBScopeTenant)
	}
}

func TestKnowledgeBaseMarshalJSONPublishesCanonicalScope(t *testing.T) {
	kb := &KnowledgeBase{ID: "kb-1", Type: KnowledgeBaseTypeDocument, Scope: "public"}
	raw, err := json.Marshal(kb)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["scope"] != KBScopeTenant {
		t.Fatalf("response scope = %v, want %q", payload["scope"], KBScopeTenant)
	}
}
