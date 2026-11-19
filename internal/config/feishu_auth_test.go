package config

import "testing"

func TestFeishuAuthEnvOverrides(t *testing.T) {
	t.Setenv("FEISHU_AUTH_APP_ID", "cli_env_app")
	t.Setenv("FEISHU_AUTH_APP_SECRET", "env_secret")
	t.Setenv("FEISHU_AUTH_REDIRECT_URI", "https://llm-ai.c-smart.hk/c-knowledge/api/v1/auth/feishu/callback")
	t.Setenv("FEISHU_AUTH_TENANT_KEY", "tenant_env")
	t.Setenv("WEKNORA_SHARED_TENANT_ID", "10000")

	cfg := &Config{}
	applyFeishuAuthOverrides(cfg)

	if cfg.FeishuAuth == nil {
		t.Fatal("FeishuAuth section was not created")
	}
	if !cfg.FeishuAuth.IsEnabled() {
		t.Fatal("FeishuAuth should be enabled with app_id/app_secret from env")
	}
	if cfg.FeishuAuth.AppID != "cli_env_app" || cfg.FeishuAuth.AppSecret != "env_secret" {
		t.Errorf("credentials = (%q, %q), want env values", cfg.FeishuAuth.AppID, cfg.FeishuAuth.AppSecret)
	}
	if cfg.FeishuAuth.RedirectURI != "https://llm-ai.c-smart.hk/c-knowledge/api/v1/auth/feishu/callback" {
		t.Errorf("RedirectURI = %q", cfg.FeishuAuth.RedirectURI)
	}
	if cfg.FeishuAuth.TenantKey != "tenant_env" {
		t.Errorf("TenantKey = %q", cfg.FeishuAuth.TenantKey)
	}
	if cfg.FeishuAuth.SharedTenantID != 10000 {
		t.Errorf("SharedTenantID = %d, want 10000", cfg.FeishuAuth.SharedTenantID)
	}
	if cfg.FeishuAuth.DefaultRole != "viewer" {
		t.Errorf("DefaultRole = %q, want viewer", cfg.FeishuAuth.DefaultRole)
	}
}

func TestFeishuAuthDisabledByDefault(t *testing.T) {
	t.Setenv("FEISHU_AUTH_APP_ID", "")
	t.Setenv("FEISHU_AUTH_APP_SECRET", "")

	cfg := &Config{}
	applyFeishuAuthOverrides(cfg)
	if cfg.FeishuAuth.IsEnabled() {
		t.Fatal("FeishuAuth must be disabled without credentials")
	}
}

func TestDefaultSharedTenantIDFallsBack(t *testing.T) {
	t.Setenv("WEKNORA_SHARED_TENANT_ID", "not-a-number")
	if got := defaultSharedTenantID(); got != 10000 {
		t.Fatalf("defaultSharedTenantID() = %d, want 10000 fallback", got)
	}
	t.Setenv("WEKNORA_SHARED_TENANT_ID", "0")
	if got := defaultSharedTenantID(); got != 10000 {
		t.Fatalf("defaultSharedTenantID() = %d, want 10000 fallback for zero", got)
	}
	t.Setenv("WEKNORA_SHARED_TENANT_ID", "42")
	if got := defaultSharedTenantID(); got != 42 {
		t.Fatalf("defaultSharedTenantID() = %d, want 42", got)
	}
}

func TestValidateConfigRejectsInvalidFeishuRole(t *testing.T) {
	cfg := &Config{FeishuAuth: &FeishuAuthConfig{
		AppID:          "cli",
		AppSecret:      "secret",
		SharedTenantID: 10000,
		DefaultRole:    "superuser",
	}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("ValidateConfig should reject an unknown feishu default_role")
	}
}
