// Package config 应用配置管理
// 从 trpc_go.yaml 的 custom 块加载配置，支持默认值回退和环境变量覆盖
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Model    ModelConfig    `yaml:"model"`
	MCP      MCPConfig      `yaml:"mcp"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Security SecurityConfig `yaml:"security"`
	Tools    ToolsConfig    `yaml:"tools"`
	RAG      RAGConfig      `yaml:"rag"`
	Memory   MemoryConfig   `yaml:"memory"`
	Cache    CacheConfig    `yaml:"cache"`
}

// CacheConfig 缓存配置（Redis）
type CacheConfig struct {
	// Enabled 是否启用缓存，默认 false
	Enabled bool `yaml:"enabled"`
	// RedisAddr Redis 地址，如 localhost:6379
	RedisAddr string `yaml:"redis_addr"`
	// RedisDB Redis 数据库编号，默认 0
	RedisDB int `yaml:"redis_db"`
	// Password Redis 密码，遵循 secrets env-only 原则，仅通过环境变量 REDIS_PASSWORD 提供，不从配置文件读取
	Password string `yaml:"-"`
	// EmbedTTL Embedding 缓存过期时间（秒），<=0 表示永不过期，默认 0
	EmbedTTL int `yaml:"embed_ttl"`
	// SemanticEnabled 是否启用 LLM 语义缓存（相似问题复用历史回答），默认 false
	SemanticEnabled bool `yaml:"semantic_enabled"`
	// SemanticThreshold 语义相似度命中阈值 [0,1]，默认 0.92
	SemanticThreshold float64 `yaml:"semantic_threshold"`
	// SemanticMaxEntries 每个 scope 最多缓存的问答数，默认 50
	SemanticMaxEntries int `yaml:"semantic_max_entries"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	HTTPPort string `yaml:"http_port"`
	MCPPort  string `yaml:"mcp_port"`
	Host     string `yaml:"host"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	// AllowedOrigins CORS 允许的来源列表，留空则拒绝所有跨域请求
	// 支持精确匹配（如 http://localhost:3000）和通配符 "*"（仅开发环境使用）
	AllowedOrigins []string `yaml:"allowed_origins"`
	// RateLimit 限流配置
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	// TrustProxyHeaders 是否信任反向代理头（X-Forwarded-For/X-Real-IP）
	// 仅当服务部署在可信反代之后时才设为 true，否则限流可被伪造头绕过
	TrustProxyHeaders bool `yaml:"trust_proxy_headers"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	// Enabled 是否启用限流，默认 true
	Enabled bool `yaml:"enabled"`
	// RequestsPerSecond 每个 IP 每秒允许的请求数（令牌桶速率），默认 10
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// Burst 令牌桶容量（突发请求上限），默认 30
	Burst int `yaml:"burst"`
}

// ModelConfig 模型配置
type ModelConfig struct {
	Name             string        `yaml:"name"`
	Type             string        `yaml:"type"`
	Timeout          int           `yaml:"timeout"`
	MaxContextLength int           `yaml:"max_context_length"`
	OllamaURL        string        `yaml:"ollama_url"`
	// OpenAI兼容接口配置（阿里云DashScope等）
	OpenAIBaseURL    string        `yaml:"openai_base_url"`
	OpenAIAPIKey     string        `yaml:"openai_api_key"`
	// 可用模型列表（前端切换用）
	AvailableModels  []ModelOption `yaml:"available_models"`
}

// ModelOption 可选模型选项
type ModelOption struct {
	Name  string `yaml:"name"`  // 模型标识，如 qwen-plus
	Label string `yaml:"label"` // 展示名称，如 通义千问 Plus
	Type  string `yaml:"type"`  // 模型类型：cloud（云端）或 local（本地Ollama），留空时自动判断
}

// ToolsConfig 工具配置
type ToolsConfig struct {
	// BaiduAK 百度地图 API Key，用于天气查询和逆地理编码
	BaiduAK string `yaml:"baidu_ak"`
}

// RAGConfig RAG 知识库配置
type RAGConfig struct {
	// EmbedModel Embedding 模型名称，默认 text-embedding-3-small
	EmbedModel string `yaml:"embed_model"`
	// Enabled 是否启用 RAG 功能，默认 true
	Enabled bool `yaml:"enabled"`
}

// MemoryConfig 长期记忆配置（独立于 RAG，可单独启用）
type MemoryConfig struct {
	// Enabled 是否启用跨会话向量记忆，默认 true
	Enabled bool `yaml:"enabled"`
	// EmbedModel Embedding 模型名称，留空则复用 RAG 的 embed_model，最终回退到默认值
	EmbedModel string `yaml:"embed_model"`
}

// MCPConfig MCP配置
type MCPConfig struct {
	Version     string `yaml:"version"`
	Enabled     bool   `yaml:"enabled"`
	ServiceName string `yaml:"service_name"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level    string `yaml:"level"`
	FilePath string `yaml:"file_path"`
	Console  bool   `yaml:"console"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	MySQL MySQLConfig `yaml:"mysql"`
}

// MySQLConfig MySQL数据库配置
type MySQLConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	Username        string `yaml:"username"`
	Password        string `yaml:"password"`
	Database        string `yaml:"database"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
}

// trpcYAML 用于解析 trpc_go.yaml 顶层结构，提取 custom 块
type trpcYAML struct {
	Custom *Config `yaml:"custom"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPPort: "8081",
			MCPPort:  "8001",
			Host:     "localhost",
		},
		Model: ModelConfig{
			Name:             "qwen-plus",
			Type:             "openai",
			Timeout:          5000,
			MaxContextLength: 4096,
			OllamaURL:        "http://localhost:11434",
			OpenAIBaseURL:    "",
			OpenAIAPIKey:     "",
			AvailableModels: []ModelOption{
				{Name: "qwen-plus", Label: "通义千问 Plus", Type: "cloud"},
				{Name: "qwen-max", Label: "通义千问 Max", Type: "cloud"},
				{Name: "qwen-turbo", Label: "通义千问 Turbo", Type: "cloud"},
			},
		},
		MCP: MCPConfig{
			Version:     "1.0",
			Enabled:     true,
			ServiceName: "ai-chat-service",
		},
		Log: LogConfig{
			Level:    "info",
			FilePath: "./logs/app.log",
			Console:  true,
		},
		Security: SecurityConfig{
			AllowedOrigins: []string{"http://localhost:8081", "http://127.0.0.1:8081"},
			RateLimit: RateLimitConfig{
				Enabled:           true,
				RequestsPerSecond: 10,
				Burst:             30,
			},
		},
		Database: DatabaseConfig{
			MySQL: MySQLConfig{
				Host:            "localhost",
				Port:            3306,
				Username:        "root",
				Password:        "", // 敏感信息不硬编码，通过环境变量 MYSQL_PASSWORD 或配置文件提供
				Database:        "ai_chat_db",
				MaxIdleConns:    10,
				MaxOpenConns:    100,
				ConnMaxLifetime: 3600,
			},
		},
		Tools: ToolsConfig{
			BaiduAK: "",
		},
		RAG: RAGConfig{
			Enabled:    true,
			EmbedModel: "text-embedding-3-small",
		},
		Memory: MemoryConfig{
			Enabled:    true,
			EmbedModel: "", // 留空时优先复用 RAG 的 embed_model
		},
		Cache: CacheConfig{
			Enabled:            false,
			RedisAddr:          "localhost:6379",
			RedisDB:            0,
			EmbedTTL:           0, // 永不过期（向量化结果确定性强，可长期缓存）
			SemanticEnabled:    false,
			SemanticThreshold:  0.92,
			SemanticMaxEntries: 50,
		},
	}
}

// LoadConfig 从 trpc_go.yaml 的 custom 块加载应用配置，缺失字段回退到默认值
func LoadConfig() *Config {
	cfg := DefaultConfig()

	// 按优先级依次查找配置文件
	candidates := []string{
		"trpc_go.yaml",
		"../trpc_go.yaml",
		"../../trpc_go.yaml",
	}
	// 也支持通过环境变量指定配置文件路径
	if envPath := os.Getenv("TRPC_CONFIG_PATH"); envPath != "" {
		candidates = append([]string{envPath}, candidates...)
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var wrapper trpcYAML
		if err := yaml.Unmarshal(data, &wrapper); err != nil || wrapper.Custom == nil {
			continue
		}
		// 将 custom 块中非零值覆盖到默认配置
		mergeConfig(cfg, wrapper.Custom)
		applyEnvOverrides(cfg)
		return cfg
	}
	applyEnvOverrides(cfg)
	return cfg
}

// applyEnvOverrides 用环境变量覆盖敏感配置（遵循 secrets env-only 原则）
// 优先级：环境变量 > 配置文件 > 默认值
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.Model.OpenAIAPIKey = v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.Model.OpenAIBaseURL = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		cfg.Database.MySQL.Password = v
	}
	if v := os.Getenv("MYSQL_USERNAME"); v != "" {
		cfg.Database.MySQL.Username = v
	}
	if v := os.Getenv("MYSQL_HOST"); v != "" {
		cfg.Database.MySQL.Host = v
	}
	if v := os.Getenv("BAIDU_AK"); v != "" {
		cfg.Tools.BaiduAK = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Cache.Password = v
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Cache.RedisAddr = v
	}
}

// mergeConfig 将 src 中非零值字段覆盖到 dst
func mergeConfig(dst, src *Config) {
	// Server
	if src.Server.HTTPPort != "" {
		dst.Server.HTTPPort = src.Server.HTTPPort
	}
	if src.Server.MCPPort != "" {
		dst.Server.MCPPort = src.Server.MCPPort
	}
	if src.Server.Host != "" {
		dst.Server.Host = src.Server.Host
	}
	// Model
	if src.Model.Name != "" {
		dst.Model.Name = src.Model.Name
	}
	if src.Model.Type != "" {
		dst.Model.Type = src.Model.Type
	}
	if src.Model.OllamaURL != "" {
		dst.Model.OllamaURL = src.Model.OllamaURL
	}
	if src.Model.OpenAIBaseURL != "" {
		dst.Model.OpenAIBaseURL = src.Model.OpenAIBaseURL
	}
	if src.Model.OpenAIAPIKey != "" {
		dst.Model.OpenAIAPIKey = src.Model.OpenAIAPIKey
	}
	if src.Model.Timeout != 0 {
		dst.Model.Timeout = src.Model.Timeout
	}
	if src.Model.MaxContextLength != 0 {
		dst.Model.MaxContextLength = src.Model.MaxContextLength
	}
	if len(src.Model.AvailableModels) > 0 {
		dst.Model.AvailableModels = src.Model.AvailableModels
	}
	// MCP
	if src.MCP.Version != "" {
		dst.MCP.Version = src.MCP.Version
	}
	if src.MCP.ServiceName != "" {
		dst.MCP.ServiceName = src.MCP.ServiceName
	}
	dst.MCP.Enabled = src.MCP.Enabled
	// Log
	if src.Log.Level != "" {
		dst.Log.Level = src.Log.Level
	}
	if src.Log.FilePath != "" {
		dst.Log.FilePath = src.Log.FilePath
	}
	dst.Log.Console = src.Log.Console
	// Security
	if len(src.Security.AllowedOrigins) > 0 {
		dst.Security.AllowedOrigins = src.Security.AllowedOrigins
	}
	if src.Security.RateLimit.RequestsPerSecond != 0 {
		dst.Security.RateLimit.RequestsPerSecond = src.Security.RateLimit.RequestsPerSecond
	}
	if src.Security.RateLimit.Burst != 0 {
		dst.Security.RateLimit.Burst = src.Security.RateLimit.Burst
	}
	dst.Security.RateLimit.Enabled = src.Security.RateLimit.Enabled
	dst.Security.TrustProxyHeaders = src.Security.TrustProxyHeaders
	// Database.MySQL
	if src.Database.MySQL.Host != "" {
		dst.Database.MySQL.Host = src.Database.MySQL.Host
	}
	if src.Database.MySQL.Port != 0 {
		dst.Database.MySQL.Port = src.Database.MySQL.Port
	}
	if src.Database.MySQL.Username != "" {
		dst.Database.MySQL.Username = src.Database.MySQL.Username
	}
	if src.Database.MySQL.Password != "" {
		dst.Database.MySQL.Password = src.Database.MySQL.Password
	}
	if src.Database.MySQL.Database != "" {
		dst.Database.MySQL.Database = src.Database.MySQL.Database
	}
	if src.Database.MySQL.MaxIdleConns != 0 {
		dst.Database.MySQL.MaxIdleConns = src.Database.MySQL.MaxIdleConns
	}
	if src.Database.MySQL.MaxOpenConns != 0 {
		dst.Database.MySQL.MaxOpenConns = src.Database.MySQL.MaxOpenConns
	}
	if src.Database.MySQL.ConnMaxLifetime != 0 {
		dst.Database.MySQL.ConnMaxLifetime = src.Database.MySQL.ConnMaxLifetime
	}
	// Tools
	if src.Tools.BaiduAK != "" {
		dst.Tools.BaiduAK = src.Tools.BaiduAK
	}
	// RAG
	if src.RAG.EmbedModel != "" {
		dst.RAG.EmbedModel = src.RAG.EmbedModel
	}
	dst.RAG.Enabled = src.RAG.Enabled
	// Memory
	if src.Memory.EmbedModel != "" {
		dst.Memory.EmbedModel = src.Memory.EmbedModel
	}
	dst.Memory.Enabled = src.Memory.Enabled
	// Cache
	if src.Cache.RedisAddr != "" {
		dst.Cache.RedisAddr = src.Cache.RedisAddr
	}
	if src.Cache.RedisDB != 0 {
		dst.Cache.RedisDB = src.Cache.RedisDB
	}
	if src.Cache.EmbedTTL != 0 {
		dst.Cache.EmbedTTL = src.Cache.EmbedTTL
	}
	if src.Cache.SemanticThreshold != 0 {
		dst.Cache.SemanticThreshold = src.Cache.SemanticThreshold
	}
	if src.Cache.SemanticMaxEntries != 0 {
		dst.Cache.SemanticMaxEntries = src.Cache.SemanticMaxEntries
	}
	dst.Cache.SemanticEnabled = src.Cache.SemanticEnabled
	dst.Cache.Enabled = src.Cache.Enabled
}