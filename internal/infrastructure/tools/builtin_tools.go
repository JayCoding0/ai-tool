// Package tools 工具加载器
// builtin_tools.go — 工具加载与注册核心（执行器实现见 builtin_tools_executors.go）
// 工具不再自动注册，而是通过扫描 skills/*/scripts/ 目录动态加载
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

const (
	// scriptExecTimeout 单次脚本执行最长超时时间
	scriptExecTimeout = 30 * time.Second
	// scriptMaxOutputBytes 脚本输出最大字节数（512KB），防止输出爆炸
	scriptMaxOutputBytes = 512 * 1024
)

// toolManifest scripts/ 目录下工具定义文件（tool.json）的结构
type toolManifest struct {
	Name        string                      `json:"name"`
	DisplayName string                      `json:"display_name"` // 展示给用户的中文名称（可选）
	Description string                      `json:"description"`
	Script      string                      `json:"script"` // 脚本文件名，如 run.sh / run.py
	Parameters  domain_model.ToolParameters `json:"parameters"`
}

// LoadToolsFromSkillsDir 扫描 skillsDir 下所有技能的 scripts/ 子目录，
// 读取 tool.json 并注册对应工具。
// 每个工具执行时会运行 scripts/ 目录下对应的脚本文件，并将参数以 JSON 形式通过 stdin 传入。
// baiduAK 为百度地图 API Key，用于天气查询和逆地理编码工具。
func LoadToolsFromSkillsDir(skillsDir string, baiduAK string) {
	logger := shared.GetLogger()

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		logger.Warn("读取 skills 目录失败，跳过工具加载", zap.String("dir", skillsDir), zap.Error(err))
		return
	}

	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		scriptsDir := filepath.Join(skillsDir, entry.Name(), "scripts")
		if _, err := os.Stat(scriptsDir); os.IsNotExist(err) {
			continue
		}
		n := loadToolsFromScriptsDir(scriptsDir, baiduAK, logger)
		loaded += n
	}

	logger.Info("工具加载完成", zap.Int("total", loaded))
}

// loadToolsFromScriptsDir 从单个 scripts/ 目录加载所有工具
func loadToolsFromScriptsDir(scriptsDir string, baiduAK string, logger *zap.Logger) int {
	manifestPath := filepath.Join(scriptsDir, "tool.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		logger.Warn("未找到 tool.json，跳过", zap.String("scripts_dir", scriptsDir))
		return 0
	}

	// 支持单个工具定义 {} 或多个工具定义 [{}]
	var manifests []toolManifest
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(data, &manifests); err != nil {
			logger.Warn("解析 tool.json 数组失败", zap.String("path", manifestPath), zap.Error(err))
			return 0
		}
	} else {
		var single toolManifest
		if err := json.Unmarshal(data, &single); err != nil {
			logger.Warn("解析 tool.json 失败", zap.String("path", manifestPath), zap.Error(err))
			return 0
		}
		manifests = []toolManifest{single}
	}

	count := 0
	for _, m := range manifests {
		if m.Name == "" {
			continue
		}
		registerScriptTool(m, scriptsDir, baiduAK, logger)
		count++
	}
	return count
}

// validateScriptPath 校验脚本路径是否在允许的 scriptsDir 白名单目录内，防止路径穿越攻击
func validateScriptPath(scriptsDir, scriptPath string) error {
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("解析脚本路径失败: %v", err)
	}
	allowedBase, err := filepath.Abs(scriptsDir)
	if err != nil {
		return fmt.Errorf("解析白名单目录失败: %v", err)
	}
	if !strings.HasPrefix(abs, allowedBase+string(filepath.Separator)) {
		return fmt.Errorf("脚本路径 %q 不在允许的目录 %q 内，拒绝执行", abs, allowedBase)
	}
	return nil
}

// registerScriptTool 注册一个脚本驱动的工具
func registerScriptTool(m toolManifest, scriptsDir string, baiduAK string, logger *zap.Logger) {
	scriptPath := filepath.Join(scriptsDir, m.Script)

	// 根据内置工具名分发到对应的 Go 实现（执行器在 builtin_tools_executors.go）
	var execFunc tool.ExecuteFunc
	switch m.Name {
	case "list_directory":
		execFunc = executeListDirectory
	case "get_weather":
		execFunc = makeWeatherExecutor(baiduAK)
	case "get_public_ip":
		execFunc = makePublicIPExecutor(baiduAK)
	default:
		// 通用：执行脚本文件，参数通过 stdin 以 JSON 传入
		// 先做路径白名单校验，防止 tool.json 中配置了恶意路径
		if err := validateScriptPath(scriptsDir, scriptPath); err != nil {
			logger.Error("脚本路径校验失败，跳过注册", zap.String("name", m.Name), zap.Error(err))
			return
		}
		execFunc = makeScriptExecutor(scriptPath, scriptsDir)
	}

	tool.Register(&tool.Tool{
		Definition: domain_model.ToolDefinition{
			Name:        m.Name,
			DisplayName: m.DisplayName,
			Description: m.Description,
			Parameters:  m.Parameters,
		},
		Execute: execFunc,
	})
	logger.Info("工具已注册", zap.String("name", m.Name), zap.String("scripts_dir", scriptsDir))
}