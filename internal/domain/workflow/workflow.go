// Package workflow 定义工作流领域的核心类型
// 包括 Workflow 聚合根、Node/Edge 值对象、DAG 拓扑排序等
package workflow

import (
	"fmt"
	"time"
)

// ─── 节点类型枚举 ──────────────────────────────────────────────────────────────

// NodeType 节点类型
type NodeType string

const (
	NodeTypeStart    NodeType = "start"    // 开始节点（入口）
	NodeTypeEnd      NodeType = "end"      // 结束节点（出口）
	NodeTypeLLM      NodeType = "llm"      // LLM 对话节点
	NodeTypeTool     NodeType = "tool"     // 工具调用节点
	NodeTypeAgent    NodeType = "agent"    // 子 Agent 节点（复用现有 AgentRegistry）
	NodeTypeTemplate NodeType = "template" // 模板转换节点（文本拼接/格式化）
	NodeTypeHTTP     NodeType = "http"     // HTTP 请求节点
)

// ─── 节点定义 ──────────────────────────────────────────────────────────────────

// Node DAG 中的一个节点
type Node struct {
	ID          string     `json:"id"`          // 节点唯一 ID（前端生成，如 "node_1"）
	Type        NodeType   `json:"type"`        // 节点类型
	Name        string     `json:"name"`        // 节点显示名称
	Description string     `json:"description"` // 节点描述
	Config      NodeConfig `json:"config"`       // 节点配置（按类型不同）
	Position    Position   `json:"position"`    // 画布坐标（前端用）
}

// Position 节点在画布上的坐标
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// NodeConfig 节点配置（按节点类型使用不同字段）
type NodeConfig struct {
	// LLM 节点配置
	ModelName    string  `json:"model_name,omitempty"`    // 使用的模型
	SystemPrompt string  `json:"system_prompt,omitempty"` // System Prompt（支持 ${变量} 模板）
	UserPrompt   string  `json:"user_prompt,omitempty"`   // 用户 Prompt 模板（支持 ${node_id.output} 引用上游输出）
	Temperature  float64 `json:"temperature,omitempty"`   // 温度参数

	// Tool 节点配置
	ToolName string            `json:"tool_name,omitempty"` // 工具名称
	ToolArgs map[string]string `json:"tool_args,omitempty"` // 工具参数模板（支持 ${node_id.output} 引用上游输出）

	// Agent 节点配置
	AgentName    string `json:"agent_name,omitempty"`    // 子 Agent 名称（复用 AgentRegistry）
	AgentMessage string `json:"agent_message,omitempty"` // 发送给子 Agent 的消息模板

	// HTTP 节点配置
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"` // 支持模板变量

	// Template 节点配置
	Template string `json:"template,omitempty"` // 模板字符串，支持 ${node_id.output}

	// 通用配置
	InputMapping map[string]string `json:"input_mapping,omitempty"` // 输入映射：本节点变量名 → 上游节点输出引用
	OutputKey    string            `json:"output_key,omitempty"`    // 输出变量名（供下游引用）
	TimeoutSec   int              `json:"timeout_sec,omitempty"`   // 超时时间（秒），0 表示使用默认值
	RetryCount   int              `json:"retry_count,omitempty"`   // 失败重试次数
}

// ─── 边定义 ──────────────────────────────────────────────────────────────────

// Edge DAG 中的一条有向边
type Edge struct {
	ID           string `json:"id"`                      // 边唯一 ID
	Source       string `json:"source"`                  // 源节点 ID
	Target       string `json:"target"`                  // 目标节点 ID
	SourceHandle string `json:"source_handle,omitempty"` // 源节点输出端口（条件分支用）
	Label        string `json:"label,omitempty"`         // 边标签
}

// ─── Workflow 聚合根 ──────────────────────────────────────────────────────────

// Status Workflow 状态
type Status string

const (
	StatusDraft     Status = "draft"     // 草稿
	StatusPublished Status = "published" // 已发布
	StatusArchived  Status = "archived"  // 已归档
)

// Workflow DAG 工作流聚合根
type Workflow struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`        // 工作流名称
	Description string     `json:"description"` // 工作流描述
	Nodes       []Node     `json:"nodes"`       // 节点列表
	Edges       []Edge     `json:"edges"`       // 边列表
	Variables   []Variable `json:"variables"`   // 全局变量定义
	Status      Status     `json:"status"`      // 状态
	Version     int        `json:"version"`     // 版本号
	UserID      int64      `json:"user_id"`     // 创建者
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Variable 工作流全局变量
type Variable struct {
	Name         string `json:"name"`          // 变量名
	Type         string `json:"type"`          // "string" | "number" | "boolean" | "json"
	DefaultValue string `json:"default_value"` // 默认值
	Description  string `json:"description"`   // 变量描述
	Required     bool   `json:"required"`      // 是否必填
}

// ─── DAG 图操作方法 ──────────────────────────────────────────────────────────

// GraphData DAG 图数据（用于 JSON 序列化存储）
type GraphData struct {
	Nodes     []Node     `json:"nodes"`
	Edges     []Edge     `json:"edges"`
	Variables []Variable `json:"variables,omitempty"`
}

// GetGraphData 获取工作流的图数据
func (w *Workflow) GetGraphData() GraphData {
	return GraphData{
		Nodes:     w.Nodes,
		Edges:     w.Edges,
		Variables: w.Variables,
	}
}

// SetGraphData 设置工作流的图数据
func (w *Workflow) SetGraphData(data GraphData) {
	w.Nodes = data.Nodes
	w.Edges = data.Edges
	w.Variables = data.Variables
}

// GetNodeByID 按 ID 获取节点
func (w *Workflow) GetNodeByID(nodeID string) (*Node, bool) {
	for i := range w.Nodes {
		if w.Nodes[i].ID == nodeID {
			return &w.Nodes[i], true
		}
	}
	return nil, false
}

// GetStartNode 获取开始节点
func (w *Workflow) GetStartNode() (*Node, bool) {
	for i := range w.Nodes {
		if w.Nodes[i].Type == NodeTypeStart {
			return &w.Nodes[i], true
		}
	}
	return nil, false
}

// GetEndNodes 获取所有结束节点
func (w *Workflow) GetEndNodes() []*Node {
	var ends []*Node
	for i := range w.Nodes {
		if w.Nodes[i].Type == NodeTypeEnd {
			ends = append(ends, &w.Nodes[i])
		}
	}
	return ends
}

// GetDownstreamNodes 获取指定节点的所有下游节点 ID
func (w *Workflow) GetDownstreamNodes(nodeID string) []string {
	var downstream []string
	for _, edge := range w.Edges {
		if edge.Source == nodeID {
			downstream = append(downstream, edge.Target)
		}
	}
	return downstream
}

// GetUpstreamNodes 获取指定节点的所有上游节点 ID
func (w *Workflow) GetUpstreamNodes(nodeID string) []string {
	var upstream []string
	for _, edge := range w.Edges {
		if edge.Target == nodeID {
			upstream = append(upstream, edge.Source)
		}
	}
	return upstream
}

// ─── 拓扑排序（Kahn 算法）──────────────────────────────────────────────────────

// TopologicalSort 对 DAG 进行拓扑排序（Kahn 算法）
// 返回按执行顺序排列的节点 ID 列表
// 如果存在环路，返回错误
func (w *Workflow) TopologicalSort() ([]string, error) {
	if len(w.Nodes) == 0 {
		return nil, fmt.Errorf("工作流没有节点")
	}

	// 构建邻接表和入度表
	nodeSet := make(map[string]bool)
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)

	for _, node := range w.Nodes {
		nodeSet[node.ID] = true
		inDegree[node.ID] = 0
		adjacency[node.ID] = nil
	}

	for _, edge := range w.Edges {
		if !nodeSet[edge.Source] || !nodeSet[edge.Target] {
			return nil, fmt.Errorf("边 %s → %s 引用了不存在的节点", edge.Source, edge.Target)
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge.Target)
		inDegree[edge.Target]++
	}

	// Kahn 算法：从入度为 0 的节点开始
	var queue []string
	for nodeID, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, nodeID)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		// 取出队首
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		// 移除当前节点的所有出边
		for _, neighbor := range adjacency[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// 如果排序结果数量不等于节点数量，说明存在环路
	if len(sorted) != len(w.Nodes) {
		return nil, fmt.Errorf("工作流存在环路，无法执行拓扑排序（已排序 %d/%d 个节点）", len(sorted), len(w.Nodes))
	}

	return sorted, nil
}

// Validate 校验工作流定义的合法性
func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("工作流名称不能为空")
	}
	if len(w.Nodes) == 0 {
		return fmt.Errorf("工作流至少需要一个节点")
	}

	// 检查是否有 start 节点
	startCount := 0
	endCount := 0
	nodeIDs := make(map[string]bool)
	for _, node := range w.Nodes {
		if nodeIDs[node.ID] {
			return fmt.Errorf("节点 ID %q 重复", node.ID)
		}
		nodeIDs[node.ID] = true

		if node.Type == NodeTypeStart {
			startCount++
		}
		if node.Type == NodeTypeEnd {
			endCount++
		}
	}

	if startCount == 0 {
		return fmt.Errorf("工作流必须有一个开始节点（start）")
	}
	if startCount > 1 {
		return fmt.Errorf("工作流只能有一个开始节点（start），当前有 %d 个", startCount)
	}
	if endCount == 0 {
		return fmt.Errorf("工作流必须有至少一个结束节点（end）")
	}

	// 检查边引用的节点是否存在
	for _, edge := range w.Edges {
		if !nodeIDs[edge.Source] {
			return fmt.Errorf("边 %q 的源节点 %q 不存在", edge.ID, edge.Source)
		}
		if !nodeIDs[edge.Target] {
			return fmt.Errorf("边 %q 的目标节点 %q 不存在", edge.ID, edge.Target)
		}
	}

	// 检查是否有环路
	if _, err := w.TopologicalSort(); err != nil {
		return err
	}

	return nil
}
