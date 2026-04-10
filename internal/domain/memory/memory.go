// Package memory 定义记忆领域的核心实体、类型枚举和值对象
// 参考 Mem0 论文的双阶段记忆管理架构（提取 → 更新）
package memory

import (
	"time"
)

// ─── 记忆类型枚举 ──────────────────────────────────────────────────────────────

// MemoryType 记忆类型
type MemoryType string

const (
	// MemoryTypeFact 事实性记忆（如"用户是Go开发者"）
	MemoryTypeFact MemoryType = "fact"
	// MemoryTypePreference 偏好记忆（如"喜欢简洁回答"）
	MemoryTypePreference MemoryType = "preference"
	// MemoryTypeEpisode 情景记忆（重要对话片段）
	MemoryTypeEpisode MemoryType = "episode"
	// MemoryTypeSummary 会话摘要归档
	MemoryTypeSummary MemoryType = "summary"
)

// ValidMemoryTypes 合法的记忆类型集合
var ValidMemoryTypes = map[MemoryType]bool{
	MemoryTypeFact:       true,
	MemoryTypePreference: true,
	MemoryTypeEpisode:    true,
	MemoryTypeSummary:    true,
}

// IsValid 检查记忆类型是否合法
func (t MemoryType) IsValid() bool {
	return ValidMemoryTypes[t]
}

// ─── 记忆实体 ──────────────────────────────────────────────────────────────────

// Memory 用户记忆实体（向量记忆 + 结构化记忆统一存储）
type Memory struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	Content         string     `json:"content"`           // 记忆内容（自然语言描述）
	Embedding       []byte     `json:"-"`                 // 向量二进制（MEDIUMBLOB，复用 knowledge_chunks 的存储方式）
	MemoryType      MemoryType `json:"memory_type"`       // 记忆类型
	SourceSessionID string     `json:"source_session_id"` // 来源会话ID
	Importance      float64    `json:"importance"`         // 重要性分数 0-1
	AccessCount     int        `json:"access_count"`       // 被检索命中次数
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ExpiredAt       *time.Time `json:"expired_at,omitempty"` // 过期时间（nil=永不过期）
}

// ScoredMemory 带相似度分数的记忆（检索结果）
type ScoredMemory struct {
	Memory *Memory `json:"memory"`
	Score  float32 `json:"score"` // 余弦相似度分数
}

// ─── Mem0 式记忆更新操作 ────────────────────────────────────────────────────────

// OperationType 记忆更新操作类型（参考 Mem0 Update Phase）
type OperationType string

const (
	// OpAdd 新增记忆（相似度 < 0.5，全新信息）
	OpAdd OperationType = "ADD"
	// OpUpdate 更新记忆（相似度 > 0.9，合并/更新已有记忆）
	OpUpdate OperationType = "UPDATE"
	// OpDelete 删除记忆（新信息与旧记忆矛盾）
	OpDelete OperationType = "DELETE"
	// OpNoop 无操作（无新信息）
	OpNoop OperationType = "NOOP"
)

// MemoryOperation 记忆更新操作（Mem0 Update Phase 的输出）
type MemoryOperation struct {
	Op              OperationType `json:"op"`                         // 操作类型
	Content         string        `json:"content,omitempty"`          // 新增/更新的内容
	MemoryType      MemoryType    `json:"memory_type,omitempty"`     // 记忆类型
	Importance      float64       `json:"importance,omitempty"`       // 重要性分数
	TargetMemoryID  int64         `json:"target_memory_id,omitempty"` // UPDATE/DELETE 时指向的已有记忆 ID
}

// ─── 记忆提取结果 ──────────────────────────────────────────────────────────────

// ExtractedMemory LLM 从对话中提取的候选记忆（Mem0 Extraction Phase 的输出）
type ExtractedMemory struct {
	Content    string     `json:"content"`     // 记忆内容
	Type       MemoryType `json:"type"`        // 记忆类型
	Importance float64    `json:"importance"`   // 重要性分数 0-1
}

// ─── 记忆衰减常量 ──────────────────────────────────────────────────────────────

const (
	// SimilarityThresholdUpdate 相似度 > 此值时执行 UPDATE（合并已有记忆）
	SimilarityThresholdUpdate float32 = 0.9
	// SimilarityThresholdAdd 相似度 < 此值时执行 ADD（新增记忆）
	SimilarityThresholdAdd float32 = 0.5
	// MinImportanceThreshold 最低重要性阈值，低于此值的记忆不存储
	MinImportanceThreshold float64 = 0.3
	// DecayFactor 记忆衰减因子（每次衰减降低的 importance 值）
	DecayFactor float64 = 0.05
	// DecayThreshold 衰减后 importance 低于此值则标记过期
	DecayThreshold float64 = 0.1
	// MaxMemoriesPerUser 每用户记忆上限
	MaxMemoriesPerUser int = 500
)
