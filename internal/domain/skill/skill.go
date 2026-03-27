package skill

import (
	"time"
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

// Skill 技能实体
type Skill struct {
	ID           SkillID      `json:"id"`
	UserID       int64        `json:"user_id"`        // >0=用户自定义
	Name         string       `json:"name"`           // 技能名称
	Description  string       `json:"description"`    // 技能描述
	Icon         string       `json:"icon"`           // 技能图标（emoji）
	Pattern      SkillPattern `json:"pattern"`        // 技能模式
	SystemPrompt string       `json:"system_prompt"`  // 核心：System Prompt 内容
	Tools        []string     `json:"tools"`          // 绑定的工具名称列表（Function Calling）
	IsPublic     bool         `json:"is_public"`      // 是否公开
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// IsOwnedBy 是否属于指定用户
func (s *Skill) IsOwnedBy(userID int64) bool {
	return s.UserID == userID
}

// HasTools 是否绑定了工具（支持 Function Calling）
func (s *Skill) HasTools() bool {
	return len(s.Tools) > 0
}