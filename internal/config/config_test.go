package config

import (
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// DefaultConfig 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.HTTPPort != "8081" {
		t.Errorf("期望默认 HTTPPort=8081，实际 %s", cfg.Server.HTTPPort)
	}
	if cfg.Server.MCPPort != "8001" {
		t.Errorf("期望默认 MCPPort=8001，实际 %s", cfg.Server.MCPPort)
	}
	if cfg.Model.Name != "qwen-plus" {
		t.Errorf("期望默认模型 qwen-plus，实际 %s", cfg.Model.Name)
	}
	if cfg.Model.Type != "openai" {
		t.Errorf("期望默认模型类型 openai，实际 %s", cfg.Model.Type)
	}
	if !cfg.MCP.Enabled {
		t.Error("MCP 默认应启用")
	}
	if !cfg.RAG.Enabled {
		t.Error("RAG 默认应启用")
	}
	if cfg.RAG.EmbedModel != "text-embedding-3-small" {
		t.Errorf("期望默认 EmbedModel=text-embedding-3-small，实际 %s", cfg.RAG.EmbedModel)
	}
	if cfg.Database.MySQL.Port != 3306 {
		t.Errorf("期望默认 MySQL 端口 3306，实际 %d", cfg.Database.MySQL.Port)
	}
	if len(cfg.Model.AvailableModels) == 0 {
		t.Error("默认可用模型列表不应为空")
	}
	if !cfg.Security.RateLimit.Enabled {
		t.Error("限流默认应启用")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// mergeConfig 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestMergeConfig_OverridesNonZeroFields(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Server: ServerConfig{
			HTTPPort: "9090",
		},
		Model: ModelConfig{
			Name: "gpt-4",
			Type: "openai",
		},
		RAG: RAGConfig{
			EmbedModel: "custom-embed",
			Enabled:    false,
		},
	}

	mergeConfig(dst, src)

	if dst.Server.HTTPPort != "9090" {
		t.Errorf("HTTPPort 应被覆盖为 9090，实际 %s", dst.Server.HTTPPort)
	}
	if dst.Model.Name != "gpt-4" {
		t.Errorf("模型名应被覆盖为 gpt-4，实际 %s", dst.Model.Name)
	}
	if dst.RAG.EmbedModel != "custom-embed" {
		t.Errorf("EmbedModel 应被覆盖为 custom-embed，实际 %s", dst.RAG.EmbedModel)
	}
	// RAG.Enabled 是 bool，src 为 false 时也应覆盖
	if dst.RAG.Enabled != false {
		t.Error("RAG.Enabled 应被覆盖为 false")
	}
}

func TestMergeConfig_PreservesDefaults(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Server: ServerConfig{
			HTTPPort: "9090",
			// MCPPort 为空，不应覆盖
		},
	}

	mergeConfig(dst, src)

	if dst.Server.MCPPort != "8001" {
		t.Errorf("MCPPort 不应被空值覆盖，期望 8001，实际 %s", dst.Server.MCPPort)
	}
	if dst.Server.Host != "localhost" {
		t.Errorf("Host 不应被空值覆盖，期望 localhost，实际 %s", dst.Server.Host)
	}
}

func TestMergeConfig_DatabaseOverride(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Database: DatabaseConfig{
			MySQL: MySQLConfig{
				Host:     "10.0.0.1",
				Port:     3307,
				Password: "newpass",
			},
		},
	}

	mergeConfig(dst, src)

	if dst.Database.MySQL.Host != "10.0.0.1" {
		t.Errorf("MySQL Host 应被覆盖，实际 %s", dst.Database.MySQL.Host)
	}
	if dst.Database.MySQL.Port != 3307 {
		t.Errorf("MySQL Port 应被覆盖，实际 %d", dst.Database.MySQL.Port)
	}
	if dst.Database.MySQL.Password != "newpass" {
		t.Errorf("MySQL Password 应被覆盖，实际 %s", dst.Database.MySQL.Password)
	}
	// 未覆盖的字段应保持默认值
	if dst.Database.MySQL.Username != "root" {
		t.Errorf("MySQL Username 不应被覆盖，期望 root，实际 %s", dst.Database.MySQL.Username)
	}
}

func TestMergeConfig_SecurityOverride(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Security: SecurityConfig{
			AllowedOrigins: []string{"https://example.com"},
			RateLimit: RateLimitConfig{
				RequestsPerSecond: 50,
				Burst:             100,
			},
		},
	}

	mergeConfig(dst, src)

	if len(dst.Security.AllowedOrigins) != 1 || dst.Security.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("AllowedOrigins 应被覆盖，实际 %v", dst.Security.AllowedOrigins)
	}
	if dst.Security.RateLimit.RequestsPerSecond != 50 {
		t.Errorf("RequestsPerSecond 应被覆盖为 50，实际 %v", dst.Security.RateLimit.RequestsPerSecond)
	}
}
