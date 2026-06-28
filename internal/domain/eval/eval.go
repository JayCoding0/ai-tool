// Package eval 定义 Agent 评估体系的核心领域模型
// 包含评测数据集、用例、评测运行、单条评测结果等实体
package eval

import "time"

// Dataset 评测数据集（一批 [输入, 期望输出] 样本的集合）
type Dataset struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UserID      int64     `json:"user_id"`
	CaseCount   int       `json:"case_count"` // 用例数量（聚合字段，非持久化）
	CreatedAt   time.Time `json:"created_at"`
}

// Case 评测用例（单条 [输入, 期望输出]）
type Case struct {
	ID        int64     `json:"id"`
	DatasetID int64     `json:"dataset_id"`
	Input     string    `json:"input"`    // 输入（用户问题）
	Expected  string    `json:"expected"` // 期望输出（标准答案，可为空表示开放式）
	CreatedAt time.Time `json:"created_at"`
}

// RunStatus 评测运行状态
type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"   // 运行中
	RunStatusCompleted RunStatus = "completed" // 已完成
	RunStatusFailed    RunStatus = "failed"    // 失败
)

// ScorerType 评分器类型
type ScorerType string

const (
	ScorerJudge    ScorerType = "judge"    // LLM-as-judge（默认，裁判模型打分）
	ScorerExact    ScorerType = "exact"    // 精确/包含匹配（适合分类、固定答案）
	ScorerSemantic ScorerType = "semantic" // 语义相似度（向量余弦，适合开放问答）
)

// Run 一次评测运行（对某数据集用指定 Agent 配置批量跑分）
type Run struct {
	ID           int64      `json:"id"`
	DatasetID    int64      `json:"dataset_id"`
	Name         string     `json:"name"`          // 运行名称（便于对比，如"v1-加强Prompt"）
	ModelName    string     `json:"model_name"`    // 被测 Agent 使用的模型
	SystemPrompt string     `json:"system_prompt"` // 被测 Agent 的 System Prompt
	Tools        []string   `json:"tools"`         // 被测 Agent 启用的工具
	Scorer       ScorerType `json:"scorer"`        // 评分器类型
	JudgeModel   string     `json:"judge_model"`   // 评分裁判模型（scorer=judge 时生效）
	Threshold    float64    `json:"threshold"`     // 通过阈值（score >= threshold 视为通过）
	Status       RunStatus  `json:"status"`
	TotalCases   int        `json:"total_cases"`
	PassedCases  int        `json:"passed_cases"`
	AvgScore     float64    `json:"avg_score"`
	ErrorMessage string     `json:"error_message,omitempty"`
	UserID       int64      `json:"user_id"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// PassRate 通过率（0-1）
func (r *Run) PassRate() float64 {
	if r.TotalCases == 0 {
		return 0
	}
	return float64(r.PassedCases) / float64(r.TotalCases)
}

// Result 单条用例的评测结果
type Result struct {
	ID        int64     `json:"id"`
	RunID     int64     `json:"run_id"`
	CaseID    int64     `json:"case_id"`
	Input     string    `json:"input"`
	Expected  string    `json:"expected"`
	Actual    string    `json:"actual"`  // Agent 实际输出
	Score     float64   `json:"score"`   // 0-1 评分
	Passed    bool      `json:"passed"`  // 是否通过
	Reason    string    `json:"reason"`  // 评分理由（裁判给出）
	LatencyMs int64     `json:"latency_ms"`
	Tokens    int       `json:"tokens"`
	CreatedAt time.Time `json:"created_at"`
}
