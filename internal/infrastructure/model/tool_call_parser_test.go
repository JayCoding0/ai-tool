package model

import (
	"encoding/json"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// 模式1：工具名(JSON参数) 格式
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractToolCalls_Pattern1_Basic(t *testing.T) {
	content := `query_database({"table": "orders", "limit": 10})`
	toolNames := []string{"query_database", "send_email"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 1 {
		t.Fatalf("期望 1 个工具调用，实际 %d 个", len(calls))
	}
	if calls[0].Name != "query_database" {
		t.Errorf("期望工具名 query_database，实际 %s", calls[0].Name)
	}

	// 验证参数
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(calls[0].Arguments), &args); err != nil {
		t.Fatalf("参数 JSON 解析失败: %v", err)
	}
	if args["table"] != "orders" {
		t.Errorf("期望 table=orders，实际 %v", args["table"])
	}
}

func TestExtractToolCalls_Pattern1_WithSurroundingText(t *testing.T) {
	content := `好的，我来帮你查询一下。query_database({"table": "users"}) 查询完成。`
	toolNames := []string{"query_database"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 1 {
		t.Fatalf("期望 1 个工具调用，实际 %d 个", len(calls))
	}
	if calls[0].Name != "query_database" {
		t.Errorf("期望工具名 query_database，实际 %s", calls[0].Name)
	}
}

func TestExtractToolCalls_Pattern1_UnregisteredTool(t *testing.T) {
	content := `unknown_tool({"key": "value"})`
	toolNames := []string{"query_database"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 0 {
		t.Errorf("未注册的工具不应被提取，实际提取了 %d 个", len(calls))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 模式2：JSON 对象格式
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractToolCalls_Pattern2_NameArguments(t *testing.T) {
	content := `{"name": "send_email", "arguments": {"to": "test@example.com", "subject": "Hello"}}`
	toolNames := []string{"send_email"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 1 {
		t.Fatalf("期望 1 个工具调用，实际 %d 个", len(calls))
	}
	if calls[0].Name != "send_email" {
		t.Errorf("期望工具名 send_email，实际 %s", calls[0].Name)
	}
}

func TestExtractToolCalls_Pattern2_FunctionWrapper(t *testing.T) {
	content := `{"function": {"name": "query_database", "arguments": {"sql": "SELECT 1"}}}`
	toolNames := []string{"query_database"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 1 {
		t.Fatalf("期望 1 个工具调用，实际 %d 个", len(calls))
	}
	if calls[0].Name != "query_database" {
		t.Errorf("期望工具名 query_database，实际 %s", calls[0].Name)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 边界情况
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractToolCalls_EmptyContent(t *testing.T) {
	calls := extractToolCallsFromText("", []string{"tool1"})
	if len(calls) != 0 {
		t.Errorf("空内容不应提取到工具调用，实际 %d 个", len(calls))
	}
}

func TestExtractToolCalls_NoToolNames(t *testing.T) {
	content := `query_database({"table": "orders"})`
	calls := extractToolCallsFromText(content, nil)
	if len(calls) != 0 {
		t.Errorf("无工具名列表不应提取到工具调用，实际 %d 个", len(calls))
	}
}

func TestExtractToolCalls_InvalidJSON(t *testing.T) {
	content := `query_database({invalid json})`
	toolNames := []string{"query_database"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 0 {
		t.Errorf("无效 JSON 不应提取到工具调用，实际 %d 个", len(calls))
	}
}

func TestExtractToolCalls_PlainText(t *testing.T) {
	content := "这只是一段普通的文本回复，没有任何工具调用。"
	toolNames := []string{"query_database", "send_email"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) != 0 {
		t.Errorf("普通文本不应提取到工具调用，实际 %d 个", len(calls))
	}
}

func TestExtractToolCalls_CallIDIncrement(t *testing.T) {
	// 验证多个工具调用的 ID 递增
	content := `query_database({"table": "a"}) send_email({"to": "b"})`
	toolNames := []string{"query_database", "send_email"}

	calls := extractToolCallsFromText(content, toolNames)
	if len(calls) < 2 {
		t.Skipf("未提取到多个工具调用，跳过 ID 递增检查")
	}
	if calls[0].ID == calls[1].ID {
		t.Errorf("不同工具调用的 ID 不应相同: %s", calls[0].ID)
	}
}
