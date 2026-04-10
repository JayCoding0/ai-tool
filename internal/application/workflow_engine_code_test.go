// Package application Code 节点和 Loop 节点单元测试
package application

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aiProject/internal/domain/workflow"
)

// ─── JS 沙箱执行测试 ──────────────────────────────────────────────────────────

func TestRunJavaScript_BasicReturn(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{
		"name": "world",
	}

	result, err := engine.runJavaScript(context.Background(), `return "Hello, " + inputs.name + "!";`, inputs, 5*time.Second)
	if err != nil {
		t.Fatalf("JS 执行失败: %v", err)
	}

	if result != "Hello, world!" {
		t.Fatalf("期望 'Hello, world!'，实际 '%v'", result)
	}
}

func TestRunJavaScript_JSONProcessing(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{"name": "Alice", "score": 90},
			map[string]interface{}{"name": "Bob", "score": 60},
			map[string]interface{}{"name": "Charlie", "score": 85},
		},
	}

	code := `
		var filtered = inputs.data.filter(function(item) {
			return item.score >= 80;
		});
		return filtered;
	`

	result, err := engine.runJavaScript(context.Background(), code, inputs, 5*time.Second)
	if err != nil {
		t.Fatalf("JS 执行失败: %v", err)
	}

	// 结果应该是 JSON 字符串
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result.(string)), &parsed); err != nil {
		t.Fatalf("解析结果失败: %v, result=%v", err, result)
	}

	if len(parsed) != 2 {
		t.Fatalf("期望过滤后 2 个元素，实际 %d 个", len(parsed))
	}
}

func TestRunJavaScript_MathOperations(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{
		"a": 10,
		"b": 20,
	}

	result, err := engine.runJavaScript(context.Background(), `return inputs.a + inputs.b;`, inputs, 5*time.Second)
	if err != nil {
		t.Fatalf("JS 执行失败: %v", err)
	}

	if result != "30" {
		t.Fatalf("期望 '30'，实际 '%v'", result)
	}
}

func TestRunJavaScript_Timeout(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{}

	// 无限循环代码，应该超时
	code := `while(true) {}`

	_, err := engine.runJavaScript(context.Background(), code, inputs, 100*time.Millisecond)
	if err == nil {
		t.Fatal("期望超时错误，但执行成功了")
	}
	t.Logf("正确检测到超时: %v", err)
}

func TestRunJavaScript_SyntaxError(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{}

	_, err := engine.runJavaScript(context.Background(), `return {{{;`, inputs, 5*time.Second)
	if err == nil {
		t.Fatal("期望语法错误，但执行成功了")
	}
	t.Logf("正确检测到语法错误: %v", err)
}

func TestRunJavaScript_NilReturn(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{}

	result, err := engine.runJavaScript(context.Background(), `// 没有 return`, inputs, 5*time.Second)
	if err != nil {
		t.Fatalf("JS 执行失败: %v", err)
	}

	// undefined 应该返回空字符串
	if result != "" {
		t.Fatalf("期望空字符串，实际 '%v'", result)
	}
}

func TestRunJavaScript_StringManipulation(t *testing.T) {
	engine := &WorkflowEngine{}
	inputs := map[string]interface{}{
		"text": "Hello World",
	}

	code := `
		var words = inputs.text.split(" ");
		return words.map(function(w) { return w.toUpperCase(); }).join("-");
	`

	result, err := engine.runJavaScript(context.Background(), code, inputs, 5*time.Second)
	if err != nil {
		t.Fatalf("JS 执行失败: %v", err)
	}

	if result != "HELLO-WORLD" {
		t.Fatalf("期望 'HELLO-WORLD'，实际 '%v'", result)
	}
}

// ─── isTruthy 测试 ──────────────────────────────────────────────────────────

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{"hello", true},
		{"", false},
		{"false", false},
		{"0", false},
		{"null", false},
		{float64(1), true},
		{float64(0), false},
		{42, true},
		{0, false},
		{map[string]interface{}{}, true},
	}

	for _, tt := range tests {
		result := isTruthy(tt.input)
		if result != tt.expected {
			t.Errorf("isTruthy(%v) = %v, 期望 %v", tt.input, result, tt.expected)
		}
	}
}

// ─── 工作流导入/导出测试 ──────────────────────────────────────────────────────

func TestWorkflowExportImportRoundTrip(t *testing.T) {
	// 构造一个包含新节点类型的工作流
	wf := &workflow.Workflow{
		Name:        "测试工作流",
		Description: "包含 Code 和 Loop 节点",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTypeStart, Name: "开始"},
			{ID: "code_1", Type: workflow.NodeTypeCode, Name: "代码处理", Config: workflow.NodeConfig{
				CodeLanguage: "javascript",
				Code:         `return inputs.data.length;`,
				CodeInputs:   map[string]string{"data": "${start.output}"},
			}},
			{ID: "loop_1", Type: workflow.NodeTypeLoop, Name: "循环处理", Config: workflow.NodeConfig{
				LoopType:    "foreach",
				LoopList:    `["a","b","c"]`,
				LoopItemVar: "item",
				LoopBody:    `return inputs.item.toUpperCase();`,
				LoopMaxIter: 50,
			}},
			{ID: "end", Type: workflow.NodeTypeEnd, Name: "结束"},
		},
		Edges: []workflow.Edge{
			{ID: "e1", Source: "start", Target: "code_1"},
			{ID: "e2", Source: "code_1", Target: "loop_1"},
			{ID: "e3", Source: "loop_1", Target: "end"},
		},
		Variables: []workflow.Variable{
			{Name: "query", Type: "string", DefaultValue: "test", Required: true},
		},
	}

	// 导出
	exportData := &WorkflowExportData{
		FormatVersion: "1.0",
		ExportedAt:    time.Now().Format("2006-01-02 15:04:05"),
		Workflow: WorkflowExportInfo{
			Name:        wf.Name,
			Description: wf.Description,
			Nodes:       wf.Nodes,
			Edges:       wf.Edges,
			Variables:   wf.Variables,
		},
	}

	// 序列化
	jsonData, err := json.Marshal(exportData)
	if err != nil {
		t.Fatalf("序列化导出数据失败: %v", err)
	}

	// 反序列化
	var imported WorkflowExportData
	if err := json.Unmarshal(jsonData, &imported); err != nil {
		t.Fatalf("反序列化导入数据失败: %v", err)
	}

	// 验证
	if imported.FormatVersion != "1.0" {
		t.Errorf("格式版本不匹配: %s", imported.FormatVersion)
	}
	if imported.Workflow.Name != "测试工作流" {
		t.Errorf("工作流名称不匹配: %s", imported.Workflow.Name)
	}
	if len(imported.Workflow.Nodes) != 4 {
		t.Errorf("节点数量不匹配: %d", len(imported.Workflow.Nodes))
	}
	if len(imported.Workflow.Edges) != 3 {
		t.Errorf("边数量不匹配: %d", len(imported.Workflow.Edges))
	}

	// 验证 Code 节点配置保留
	codeNode := imported.Workflow.Nodes[1]
	if codeNode.Config.CodeLanguage != "javascript" {
		t.Errorf("Code 节点语言不匹配: %s", codeNode.Config.CodeLanguage)
	}
	if codeNode.Config.Code != `return inputs.data.length;` {
		t.Errorf("Code 节点代码不匹配: %s", codeNode.Config.Code)
	}

	// 验证 Loop 节点配置保留
	loopNode := imported.Workflow.Nodes[2]
	if loopNode.Config.LoopType != "foreach" {
		t.Errorf("Loop 节点类型不匹配: %s", loopNode.Config.LoopType)
	}
	if loopNode.Config.LoopMaxIter != 50 {
		t.Errorf("Loop 节点最大迭代次数不匹配: %d", loopNode.Config.LoopMaxIter)
	}
}
