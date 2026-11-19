package service

import (
	"context"
	"errors"
	"testing"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// feishuFakeUserRepo is an in-memory UserRepository covering exactly the
// lookups used by the Feishu provisioning path.
type feishuFakeUserRepo struct {
	interfaces.UserRepository
	byID       map[string]*types.User
	byOpenID   map[string]*types.User
	byEmail    map[string]*types.User
	byUsername map[string]*types.User
	createErr  error
}

func newFeishuFakeUserRepo(users ...*types.User) *feishuFakeUserRepo {
	r := &feishuFakeUserRepo{
		byID:       map[string]*types.User{},
		byOpenID:   map[string]*types.User{},
		byEmail:    map[string]*types.User{},
		byUsername: map[string]*types.User{},
	}
	for _, u := range users {
		r.byID[u.ID] = u
		if u.FeishuOpenID != nil && *u.FeishuOpenID != "" {
			r.byOpenID[*u.FeishuOpenID] = u
		}
		r.byEmail[u.Email] = u
		r.byUsername[u.Username] = u
	}
	return r
}

func (r *feishuFakeUserRepo) GetUserByFeishuOpenID(_ context.Context, openID string) (*types.User, error) {
	if u, ok := r.byOpenID[openID]; ok {
		return u, nil
	}
	return nil, apprepo.ErrUserNotFound
}

func (r *feishuFakeUserRepo) GetUserByEmail(_ context.Context, email string) (*types.User, error) {
	if u, ok := r.byEmail[email]; ok {
		return u, nil
	}
	return nil, apprepo.ErrUserNotFound
}

func (r *feishuFakeUserRepo) GetUserByUsername(_ context.Context, username string) (*types.User, error) {
	if u, ok := r.byUsername[username]; ok {
		return u, nil
	}
	return nil, apprepo.ErrUserNotFound
}

func (r *feishuFakeUserRepo) CreateUser(_ context.Context, u *types.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u
	r.byUsername[u.Username] = u
	if u.FeishuOpenID != nil && *u.FeishuOpenID != "" {
		r.byOpenID[*u.FeishuOpenID] = u
	}
	return nil
}

func (r *feishuFakeUserRepo) UpdateUser(_ context.Context, u *types.User) error {
	r.byID[u.ID] = u
	r.byEmail[u.Email] = u
	r.byUsername[u.Username] = u
	if u.FeishuOpenID != nil && *u.FeishuOpenID != "" {
		r.byOpenID[*u.FeishuOpenID] = u
	}
	return nil
}

func TestFeishuUsernameBase(t *testing.T) {
	cases := []struct {
		name string
		info *FeishuUserInfo
		want string
	}{
		{
			name: "email local part wins",
			info: &FeishuUserInfo{OpenID: "ou_0123456789", Email: "Alice.Wang@Corp.com", EnName: "alice"},
			want: "alice.wang",
		},
		{
			name: "en_name when no email",
			info: &FeishuUserInfo{OpenID: "ou_0123456789", EnName: "Wei Zhang"},
			want: "wei-zhang",
		},
		{
			name: "open_id suffix fallback",
			info: &FeishuUserInfo{OpenID: "ou_0123456789"},
			want: "fs-0123456789",
		},
		{
			name: "open_id with punctuation is compacted",
			info: &FeishuUserInfo{OpenID: "ou_ab-cd_ef-0123456789"},
			want: "fs-0123456789",
		},
		{
			name: "empty open_id falls back to fs-user",
			info: &FeishuUserInfo{},
			want: "fs-user",
		},
		{
			name: "nil info is safe",
			info: nil,
			want: "fs-user",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := feishuUsernameBase(tc.info); got != tc.want {
				t.Fatalf("feishuUsernameBase() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFeishuAccountEmail(t *testing.T) {
	cases := []struct {
		name     string
		info     *FeishuUserInfo
		username string
		want     string
	}{
		{
			name:     "personal email preferred",
			info:     &FeishuUserInfo{Email: "Alice@Corp.com", EnterpriseEmail: "alice@enterprise.com"},
			username: "alice",
			want:     "alice@corp.com",
		},
		{
			name:     "enterprise email fallback",
			info:     &FeishuUserInfo{EnterpriseEmail: "Alice@Enterprise.com"},
			username: "alice",
			want:     "alice@enterprise.com",
		},
		{
			name:     "default domain when feishu has no email",
			info:     &FeishuUserInfo{},
			username: "fs-0123456789",
			want:     "fs-0123456789@dsh.c-smart.hk",
		},
		{
			name:     "invalid feishu email ignored",
			info:     &FeishuUserInfo{Email: "not-an-email"},
			username: "alice",
			want:     "alice@dsh.c-smart.hk",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := feishuAccountEmail(tc.info, tc.username); got != tc.want {
				t.Fatalf("feishuAccountEmail() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerifyFeishuTenantKey(t *testing.T) {
	info := &FeishuUserInfo{TenantKey: "tenant-a"}
	if err := VerifyFeishuTenantKey(info, ""); err != nil {
		t.Fatalf("empty configuration should skip the check, got %v", err)
	}
	if err := VerifyFeishuTenantKey(info, "tenant-a"); err != nil {
		t.Fatalf("matching tenant_key rejected: %v", err)
	}
	if err := VerifyFeishuTenantKey(info, "tenant-b"); !errors.Is(err, ErrFeishuTenantMismatch) {
		t.Fatalf("mismatched tenant_key error = %v, want ErrFeishuTenantMismatch", err)
	}
	if err := VerifyFeishuTenantKey(nil, "tenant-a"); !errors.Is(err, ErrFeishuTenantMismatch) {
		t.Fatalf("nil info error = %v, want ErrFeishuTenantMismatch", err)
	}
}

func TestFeishuAuthConfigDisabledWithoutCredentials(t *testing.T) {
	svc := &userService{config: &config.Config{FeishuAuth: &config.FeishuAuthConfig{}}}
	if _, err := svc.getFeishuAuthConfig(); !errors.Is(err, ErrFeishuAuthDisabled) {
		t.Fatalf("error = %v, want ErrFeishuAuthDisabled", err)
	}

	svc = &userService{config: &config.Config{}}
	if _, err := svc.getFeishuAuthConfig(); !errors.Is(err, ErrFeishuAuthDisabled) {
		t.Fatalf("nil config error = %v, want ErrFeishuAuthDisabled", err)
	}
}

func TestFeishuAuthConfigDefaults(t *testing.T) {
	svc := &userService{config: &config.Config{FeishuAuth: &config.FeishuAuthConfig{
		AppID:     "cli_test",
		AppSecret: "secret",
	}}}
	cfg, err := svc.getFeishuAuthConfig()
	if err != nil {
		t.Fatalf("getFeishuAuthConfig: %v", err)
	}
	if cfg.SharedTenantID != 10000 {
		t.Errorf("SharedTenantID = %d, want 10000", cfg.SharedTenantID)
	}
	if cfg.DefaultRole != "viewer" {
		t.Errorf("DefaultRole = %q, want viewer", cfg.DefaultRole)
	}
}

func TestFindOrProvisionFeishuUserCreatesAccount(t *testing.T) {
	repo := newFeishuFakeUserRepo()
	svc := &userService{userRepo: repo}

	user, isNew, err := svc.findOrProvisionFeishuUser(context.Background(), &FeishuUserInfo{
		OpenID: "ou_0123456789",
		Name:   "Alice Wang",
		Email:  "Alice.Wang@corp.com",
	})
	if err != nil {
		t.Fatalf("findOrProvisionFeishuUser: %v", err)
	}
	if !isNew {
		t.Fatal("isNew = false, want true for a brand-new identity")
	}
	if user.Username != "alice.wang" {
		t.Errorf("Username = %q, want alice.wang", user.Username)
	}
	if user.Email != "alice.wang@corp.com" {
		t.Errorf("Email = %q, want alice.wang@corp.com", user.Email)
	}
	if user.FeishuOpenID == nil || *user.FeishuOpenID != "ou_0123456789" {
		t.Errorf("FeishuOpenID = %v, want ou_0123456789", user.FeishuOpenID)
	}
	if user.Preferences.OidcOnlyLogin == nil || !*user.Preferences.OidcOnlyLogin {
		t.Error("OidcOnlyLogin should be set for auto-provisioned accounts")
	}
	if user.TenantID != 0 {
		t.Errorf("auto-provisioned user TenantID = %d, want tenantless 0", user.TenantID)
	}
	if repo.byOpenID["ou_0123456789"] != user {
		t.Error("feishu_open_id binding was not persisted")
	}
}

func TestFindOrProvisionFeishuUserReusesOpenID(t *testing.T) {
	openID := "ou_existing"
	existing := &types.User{ID: "u1", Username: "alice", Email: "old@corp.com", FeishuOpenID: &openID, IsActive: true}
	repo := newFeishuFakeUserRepo(existing)
	svc := &userService{userRepo: repo}

	user, isNew, err := svc.findOrProvisionFeishuUser(context.Background(), &FeishuUserInfo{
		OpenID: openID,
		Email:  "new@corp.com",
	})
	if err != nil {
		t.Fatalf("findOrProvisionFeishuUser: %v", err)
	}
	if isNew || user.ID != "u1" {
		t.Fatalf("got (isNew=%v, id=%s), want existing u1", isNew, user.ID)
	}
}

func TestFindOrProvisionFeishuUserBindsExistingEmailAccount(t *testing.T) {
	existing := &types.User{ID: "u1", Username: "alice", Email: "alice@corp.com", IsActive: true}
	repo := newFeishuFakeUserRepo(existing)
	svc := &userService{userRepo: repo}

	user, isNew, err := svc.findOrProvisionFeishuUser(context.Background(), &FeishuUserInfo{
		OpenID: "ou_alice",
		Email:  "alice@corp.com",
	})
	if err != nil {
		t.Fatalf("findOrProvisionFeishuUser: %v", err)
	}
	if isNew || user.ID != "u1" {
		t.Fatalf("got (isNew=%v, id=%s), want existing u1", isNew, user.ID)
	}
	if user.FeishuOpenID == nil || *user.FeishuOpenID != "ou_alice" {
		t.Errorf("FeishuOpenID = %v, want ou_alice", user.FeishuOpenID)
	}
}

func TestFindOrProvisionFeishuUserRejectsForeignBinding(t *testing.T) {
	other := "ou_someone_else"
	existing := &types.User{ID: "u1", Username: "alice", Email: "alice@corp.com", FeishuOpenID: &other}
	repo := newFeishuFakeUserRepo(existing)
	svc := &userService{userRepo: repo}

	_, _, err := svc.findOrProvisionFeishuUser(context.Background(), &FeishuUserInfo{
		OpenID: "ou_alice",
		Email:  "alice@corp.com",
	})
	if !errors.Is(err, ErrFeishuIdentityConflict) {
		t.Fatalf("error = %v, want ErrFeishuIdentityConflict", err)
	}
}

func TestFindOrProvisionFeishuUserSuffixesConflictingUsername(t *testing.T) {
	taken := &types.User{ID: "u1", Username: "alice", Email: "someone-else@corp.com"}
	repo := newFeishuFakeUserRepo(taken)
	svc := &userService{userRepo: repo}

	user, isNew, err := svc.findOrProvisionFeishuUser(context.Background(), &FeishuUserInfo{
		OpenID: "ou_alice",
		Email:  "alice@corp.com",
	})
	if err != nil {
		t.Fatalf("findOrProvisionFeishuUser: %v", err)
	}
	if !isNew {
		t.Fatal("isNew = false, want true")
	}
	if user.Username != "alice-2" {
		t.Errorf("Username = %q, want alice-2", user.Username)
	}
	// The Feishu-provided email is authoritative and must not be rewritten.
	if user.Email != "alice@corp.com" {
		t.Errorf("Email = %q, want alice@corp.com", user.Email)
	}
}

func TestFindOrProvisionFeishuUserUsesDefaultEmailDomain(t *testing.T) {
	repo := newFeishuFakeUserRepo()
	svc := &userService{userRepo: repo}

	user, _, err := svc.findOrProvisionFeishuUser(context.Background(), &FeishuUserInfo{
		OpenID: "ou_0123456789",
	})
	if err != nil {
		t.Fatalf("findOrProvisionFeishuUser: %v", err)
	}
	if user.Email != "fs-0123456789@dsh.c-smart.hk" {
		t.Errorf("Email = %q, want fs-0123456789@dsh.c-smart.hk", user.Email)
	}
}

type feishuFakeMemberService struct {
	interfaces.TenantMemberService
	existing  *types.TenantMember
	addedRole types.TenantRole
	addCalls  int
}

func (s *feishuFakeMemberService) GetMembership(context.Context, string, uint64) (*types.TenantMember, error) {
	return s.existing, nil
}

func (s *feishuFakeMemberService) AddMember(
	_ context.Context, _ string, _ uint64, role types.TenantRole, _ *string,
) (*types.TenantMember, error) {
	s.addCalls++
	s.addedRole = role
	return &types.TenantMember{Status: types.TenantMemberStatusActive}, nil
}

func TestEnsureFeishuSharedMembershipAddsDefaultViewer(t *testing.T) {
	members := &feishuFakeMemberService{}
	svc := &userService{memberService: members}
	cfg := &config.FeishuAuthConfig{SharedTenantID: 10000, DefaultRole: "viewer"}

	if err := svc.ensureFeishuSharedMembership(context.Background(), &types.User{ID: "u1"}, cfg); err != nil {
		t.Fatalf("ensureFeishuSharedMembership: %v", err)
	}
	if members.addCalls != 1 {
		t.Fatalf("AddMember calls = %d, want 1", members.addCalls)
	}
	if members.addedRole != types.TenantRoleViewer {
		t.Fatalf("role = %q, want viewer", members.addedRole)
	}
}

func TestEnsureFeishuSharedMembershipSkipsExistingActiveMember(t *testing.T) {
	members := &feishuFakeMemberService{
		existing: &types.TenantMember{Status: types.TenantMemberStatusActive},
	}
	svc := &userService{memberService: members}
	cfg := &config.FeishuAuthConfig{SharedTenantID: 10000, DefaultRole: "viewer"}

	if err := svc.ensureFeishuSharedMembership(context.Background(), &types.User{ID: "u1"}, cfg); err != nil {
		t.Fatalf("ensureFeishuSharedMembership: %v", err)
	}
	if members.addCalls != 0 {
		t.Fatalf("AddMember calls = %d, want 0 for an existing member", members.addCalls)
	}
}

func TestLoginWithFeishuDisabled(t *testing.T) {
	svc := &userService{config: &config.Config{}}
	if _, err := svc.LoginWithFeishu(context.Background(), "code", "https://x/callback"); !errors.Is(err, ErrFeishuAuthDisabled) {
		t.Fatalf("error = %v, want ErrFeishuAuthDisabled", err)
	}
}
