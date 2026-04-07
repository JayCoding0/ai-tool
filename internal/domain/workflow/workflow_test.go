package workflow

import (
	"testing"
)

// TestTopologicalSort_LinearDAG 测试线性 DAG 的拓扑排序
func TestTopologicalSort_LinearDAG(t *testing.T) {
	wf := &Workflow{
		Name: "线性工作流",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "llm_1", Type: NodeTypeLLM, Name: "LLM 节点"},
			{ID: "tool_1", Type: NodeTypeTool, Name: "工具节点"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "llm_1"},
			{ID: "e2", Source: "llm_1", Target: "tool_1"},
			{ID: "e3", Source: "tool_1", Target: "end"},
		},
	}

	sorted, err := wf.TopologicalSort()
	if err != nil {
		t.Fatalf("拓扑排序失败: %v", err)
	}

	if len(sorted) != 4 {
		t.Fatalf("期望 4 个节点，实际 %d 个", len(sorted))
	}

	// 验证顺序：start 必须在 llm_1 之前，llm_1 必须在 tool_1 之前，tool_1 必须在 end 之前
	indexOf := func(id string) int {
		for i, s := range sorted {
			if s == id {
				return i
			}
		}
		return -1
	}

	if indexOf("start") >= indexOf("llm_1") {
		t.Error("start 应该在 llm_1 之前")
	}
	if indexOf("llm_1") >= indexOf("tool_1") {
		t.Error("llm_1 应该在 tool_1 之前")
	}
	if indexOf("tool_1") >= indexOf("end") {
		t.Error("tool_1 应该在 end 之前")
	}
}

// TestTopologicalSort_CycleDetection 测试环路检测
func TestTopologicalSort_CycleDetection(t *testing.T) {
	wf := &Workflow{
		Name: "有环工作流",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "a", Type: NodeTypeLLM, Name: "A"},
			{ID: "b", Type: NodeTypeLLM, Name: "B"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "a"},
			{ID: "e2", Source: "a", Target: "b"},
			{ID: "e3", Source: "b", Target: "a"}, // 环路！
			{ID: "e4", Source: "b", Target: "end"},
		},
	}

	_, err := wf.TopologicalSort()
	if err == nil {
		t.Fatal("期望检测到环路，但拓扑排序成功了")
	}
	t.Logf("正确检测到环路: %v", err)
}

// TestValidate_ValidWorkflow 测试合法工作流的校验
func TestValidate_ValidWorkflow(t *testing.T) {
	wf := &Workflow{
		Name: "合法工作流",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "llm_1", Type: NodeTypeLLM, Name: "LLM"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "llm_1"},
			{ID: "e2", Source: "llm_1", Target: "end"},
		},
	}

	if err := wf.Validate(); err != nil {
		t.Fatalf("合法工作流校验失败: %v", err)
	}
}

// TestValidate_NoStartNode 测试缺少开始节点
func TestValidate_NoStartNode(t *testing.T) {
	wf := &Workflow{
		Name: "无开始节点",
		Nodes: []Node{
			{ID: "llm_1", Type: NodeTypeLLM, Name: "LLM"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "llm_1", Target: "end"},
		},
	}

	err := wf.Validate()
	if err == nil {
		t.Fatal("期望校验失败（缺少 start 节点），但校验通过了")
	}
	t.Logf("正确检测到错误: %v", err)
}

// TestValidate_NoEndNode 测试缺少结束节点
func TestValidate_NoEndNode(t *testing.T) {
	wf := &Workflow{
		Name: "无结束节点",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "llm_1", Type: NodeTypeLLM, Name: "LLM"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "llm_1"},
		},
	}

	err := wf.Validate()
	if err == nil {
		t.Fatal("期望校验失败（缺少 end 节点），但校验通过了")
	}
	t.Logf("正确检测到错误: %v", err)
}

// TestValidate_DuplicateNodeID 测试重复节点 ID
func TestValidate_DuplicateNodeID(t *testing.T) {
	wf := &Workflow{
		Name: "重复节点ID",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "start", Type: NodeTypeLLM, Name: "重复"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "end"},
		},
	}

	err := wf.Validate()
	if err == nil {
		t.Fatal("期望校验失败（重复节点 ID），但校验通过了")
	}
	t.Logf("正确检测到错误: %v", err)
}

// TestValidate_EmptyName 测试空名称
func TestValidate_EmptyName(t *testing.T) {
	wf := &Workflow{
		Name: "",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "end"},
		},
	}

	err := wf.Validate()
	if err == nil {
		t.Fatal("期望校验失败（空名称），但校验通过了")
	}
	t.Logf("正确检测到错误: %v", err)
}

// TestGetDownstreamNodes 测试获取下游节点
func TestGetDownstreamNodes(t *testing.T) {
	wf := &Workflow{
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{ID: "a", Type: NodeTypeLLM},
			{ID: "b", Type: NodeTypeTool},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "a"},
			{ID: "e2", Source: "start", Target: "b"},
			{ID: "e3", Source: "a", Target: "end"},
			{ID: "e4", Source: "b", Target: "end"},
		},
	}

	downstream := wf.GetDownstreamNodes("start")
	if len(downstream) != 2 {
		t.Fatalf("期望 start 有 2 个下游节点，实际 %d 个", len(downstream))
	}
}

// TestGetUpstreamNodes 测试获取上游节点
func TestGetUpstreamNodes(t *testing.T) {
	wf := &Workflow{
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{ID: "a", Type: NodeTypeLLM},
			{ID: "b", Type: NodeTypeTool},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "e1", Source: "a", Target: "end"},
			{ID: "e2", Source: "b", Target: "end"},
		},
	}

	upstream := wf.GetUpstreamNodes("end")
	if len(upstream) != 2 {
		t.Fatalf("期望 end 有 2 个上游节点，实际 %d 个", len(upstream))
	}
}

// TestTopologicalSort_DiamondDAG 测试菱形 DAG（分叉后汇合）
func TestTopologicalSort_DiamondDAG(t *testing.T) {
	// start → a → c → end
	// start → b → c → end
	wf := &Workflow{
		Name: "菱形工作流",
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Name: "开始"},
			{ID: "a", Type: NodeTypeLLM, Name: "分支A"},
			{ID: "b", Type: NodeTypeTool, Name: "分支B"},
			{ID: "c", Type: NodeTypeTemplate, Name: "汇合"},
			{ID: "end", Type: NodeTypeEnd, Name: "结束"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "start", Target: "a"},
			{ID: "e2", Source: "start", Target: "b"},
			{ID: "e3", Source: "a", Target: "c"},
			{ID: "e4", Source: "b", Target: "c"},
			{ID: "e5", Source: "c", Target: "end"},
		},
	}

	sorted, err := wf.TopologicalSort()
	if err != nil {
		t.Fatalf("菱形 DAG 拓扑排序失败: %v", err)
	}

	if len(sorted) != 5 {
		t.Fatalf("期望 5 个节点，实际 %d 个", len(sorted))
	}

	indexOf := func(id string) int {
		for i, s := range sorted {
			if s == id {
				return i
			}
		}
		return -1
	}

	// start 必须在 a 和 b 之前
	if indexOf("start") >= indexOf("a") || indexOf("start") >= indexOf("b") {
		t.Error("start 应该在 a 和 b 之前")
	}
	// a 和 b 必须在 c 之前
	if indexOf("a") >= indexOf("c") || indexOf("b") >= indexOf("c") {
		t.Error("a 和 b 应该在 c 之前")
	}
	// c 必须在 end 之前
	if indexOf("c") >= indexOf("end") {
		t.Error("c 应该在 end 之前")
	}

	t.Logf("菱形 DAG 拓扑排序结果: %v", sorted)
}
