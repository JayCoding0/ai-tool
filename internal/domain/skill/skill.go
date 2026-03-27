package skill

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SkillID 技能ID
type SkillID int64

// SkillPattern 技能模式（对应文章中的5种设计模式）
type SkillPattern string

const (
	PatternToolWrapper SkillPattern = "tool-wrapper" // 工具封装：按需加载知识
	PatternGenerator   SkillPattern = "generator"    // 生成器：固定输出结构
	PatternReviewer    SkillPattern = "reviewer"      // 评审器：解耦检查规则
	PatternInversion   SkillPattern = "inversion"    // 倒置：先问清楚再做
	PatternPipeline    SkillPattern = "pipeline"     // 流水线：强制分步执行
)

// Skill 技能实体（对应 SKILL.md 概念）
type Skill struct {
	ID           SkillID      `json:"id"`
	UserID       int64        `json:"user_id"`        // 0=系统预设，>0=用户自定义
	Name         string       `json:"name"`           // 技能名称
	Description  string       `json:"description"`    // 技能描述
	Icon         string       `json:"icon"`           // 技能图标（emoji）
	Pattern      SkillPattern `json:"pattern"`        // 技能模式
	SystemPrompt string       `json:"system_prompt"`  // 核心：SKILL.md 的内容（含 references/assets 内容）
	Tools        []string     `json:"tools"`          // 绑定的工具名称列表（Function Calling）
	IsPublic     bool         `json:"is_public"`      // 是否公开
	SourceDir    string       `json:"source_dir,omitempty"` // 来源目录（从文件系统加载时）
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// IsSystem 是否为系统预设技能
func (s *Skill) IsSystem() bool {
	return s.UserID == 0
}

// IsOwnedBy 是否属于指定用户
func (s *Skill) IsOwnedBy(userID int64) bool {
	return s.UserID == userID
}

// IsVisibleTo 是否对指定用户可见（系统预设、公开、或自己的）
func (s *Skill) IsVisibleTo(userID int64) bool {
	return s.IsSystem() || s.IsPublic || s.IsOwnedBy(userID)
}

// HasTools 是否绑定了工具（支持 Function Calling）
func (s *Skill) HasTools() bool {
	return len(s.Tools) > 0
}

// skillFileMeta SKILL.md 文件头部 YAML 元数据
type skillFileMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Icon        string   `yaml:"icon"`
	Pattern     string   `yaml:"pattern"`
	Tools       []string `yaml:"tools"`
	Version     string   `yaml:"version"`
}

// LoadFromDirectory 从目录加载 Skill（读取 SKILL.md + references/ + assets/）
// dirPath: 技能目录路径，如 skills/数据分析助手
func LoadFromDirectory(dirPath string) (*Skill, error) {
	skillMDPath := filepath.Join(dirPath, "SKILL.md")
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return nil, err
	}

	raw := string(content)
	meta, body := parseSkillMD(raw)

	// 加载 references/ 目录下的所有 .md 文件，追加到 system_prompt
	refsDir := filepath.Join(dirPath, "references")
	if entries, err := os.ReadDir(refsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				refContent, err := os.ReadFile(filepath.Join(refsDir, entry.Name()))
				if err == nil {
					body += "\n\n---\n## 参考资料：" + entry.Name() + "\n\n" + string(refContent)
				}
			}
		}
	}

	// 加载 assets/ 目录下的所有 .md 文件，追加到 system_prompt
	assetsDir := filepath.Join(dirPath, "assets")
	if entries, err := os.ReadDir(assetsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				assetContent, err := os.ReadFile(filepath.Join(assetsDir, entry.Name()))
				if err == nil {
					body += "\n\n---\n## 模板：" + entry.Name() + "\n\n" + string(assetContent)
				}
			}
		}
	}

	name := meta.Name
	if name == "" {
		name = filepath.Base(dirPath)
	}
	icon := meta.Icon
	if icon == "" {
		icon = "🤖"
	}

	return &Skill{
		UserID:       0, // 从文件系统加载的视为系统预设
		Name:         name,
		Description:  meta.Description,
		Icon:         icon,
		Pattern:      SkillPattern(meta.Pattern),
		SystemPrompt: strings.TrimSpace(body),
		Tools:        meta.Tools,
		IsPublic:     true,
		SourceDir:    dirPath,
	}, nil
}

// parseSkillMD 解析 SKILL.md 文件，分离 YAML front matter 和正文
func parseSkillMD(content string) (skillFileMeta, string) {
	var meta skillFileMeta

	// 检查是否有 YAML front matter（以 --- 开头）
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return meta, content
	}

	// 找到第二个 ---
	lines := strings.Split(content, "\n")
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return meta, content
	}

	yamlContent := strings.Join(lines[1:endIdx], "\n")
	body := strings.Join(lines[endIdx+1:], "\n")

	yaml.Unmarshal([]byte(yamlContent), &meta) //nolint:errcheck
	return meta, body
}