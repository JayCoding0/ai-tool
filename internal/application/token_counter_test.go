package application

import (
	"testing"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
)

// ─── Token 计数器测试 ─────────────────────────────────────────────────────────

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokens(""); got != 0 {
		t.Errorf("空字符串应返回 0，实际 %d", got)
	}
}

func TestEstimateTokens_Chinese(t *testing.T) {
	// "你好世界" = 4 个 rune，估算 4*2/3 ≈ 2
	tokens := EstimateTokens("你好世界")
	if tokens < 1 {
		t.Errorf("中文文本 token 估算不应为 0，实际 %d", tokens)
	}
}

func TestEstimateTokens_English(t *testing.T) {
	tokens := EstimateTokens("Hello World, this is a test message for token counting.")
	if tokens < 10 {
		t.Errorf("英文文本 token 估算过低，实际 %d", tokens)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleSystem, Content: "你是一个智能助手"},
		{Role: model.RoleUser, Content: "你好"},
		{Role: model.RoleAssistant, Content: "你好！有什么可以帮你的？"},
	}
	tokens := EstimateMessagesTokens(messages)
	if tokens < 10 {
		t.Errorf("消息列表 token 估算过低，实际 %d", tokens)
	}
}

// ─── Token 预算计算测试 ───────────────────────────────────────────────────────

func TestCalculateTokenBudget_Default(t *testing.T) {
	budget := CalculateTokenBudget(0, "你是一个助手", "", "")
	if budget.MaxContextTokens != DefaultMaxContextTokens {
		t.Errorf("默认 MaxContextTokens 应为 %d，实际 %d", DefaultMaxContextTokens, budget.MaxContextTokens)
	}
	if budget.HistoryBudget <= 0 {
		t.Errorf("HistoryBudget 应大于 0，实际 %d", budget.HistoryBudget)
	}
}

func TestCalculateTokenBudget_WithSummary(t *testing.T) {
	budgetNoSummary := CalculateTokenBudget(8192, "你是一个助手", "", "")
	budgetWithSummary := CalculateTokenBudget(8192, "你是一个助手", "", "这是一段很长的摘要内容，包含了之前对话的关键信息。")
	if budgetWithSummary.HistoryBudget >= budgetNoSummary.HistoryBudget {
		t.Errorf("有摘要时 HistoryBudget 应更小：无摘要=%d，有摘要=%d", budgetNoSummary.HistoryBudget, budgetWithSummary.HistoryBudget)
	}
}

func TestCalculateTokenBudget_MinHistoryBudget(t *testing.T) {
	// 极端情况：System Prompt 非常长，几乎占满所有预算
	longPrompt := make([]byte, 20000)
	for i := range longPrompt {
		longPrompt[i] = 'a'
	}
	budget := CalculateTokenBudget(1000, string(longPrompt), "", "")
	if budget.HistoryBudget < 200 {
		t.Errorf("HistoryBudget 最小应为 200，实际 %d", budget.HistoryBudget)
	}
}

// ─── 历史消息裁剪测试 ─────────────────────────────────────────────────────────

func TestTrimHistoryByTokenBudget_NoTrim(t *testing.T) {
	history := []session.Message{
		{Role: "user", Content: "你好"},
		{Role: "ai", Content: "你好！"},
	}
	kept, evicted := TrimHistoryByTokenBudget(history, 10000)
	if len(kept) != 2 {
		t.Errorf("不应裁剪，期望保留 2 条，实际 %d", len(kept))
	}
	if len(evicted) != 0 {
		t.Errorf("不应有被裁剪的消息，实际 %d", len(evicted))
	}
}

func TestTrimHistoryByTokenBudget_Trim(t *testing.T) {
	history := make([]session.Message, 50)
	for i := range history {
		if i%2 == 0 {
			history[i] = session.Message{Role: "user", Content: "这是一条比较长的用户消息，用于测试 token 预算裁剪功能。"}
		} else {
			history[i] = session.Message{Role: "ai", Content: "这是一条比较长的 AI 回复消息，用于测试 token 预算裁剪功能。"}
		}
	}
	// 给一个较小的预算，应该会裁剪掉一些旧消息
	kept, evicted := TrimHistoryByTokenBudget(history, 200)
	if len(kept)+len(evicted) != 50 {
		t.Errorf("kept + evicted 应等于原始消息数 50，实际 %d + %d = %d", len(kept), len(evicted), len(kept)+len(evicted))
	}
	if len(evicted) == 0 {
		t.Error("预算较小时应有消息被裁剪")
	}
	// 保留的应该是最新的消息
	if len(kept) > 0 && kept[len(kept)-1].Content != history[49].Content {
		t.Error("保留的最后一条消息应该是原始历史的最后一条")
	}
}

func TestTrimHistoryByTokenBudget_Empty(t *testing.T) {
	kept, evicted := TrimHistoryByTokenBudget(nil, 1000)
	if kept != nil || evicted != nil {
		t.Error("空历史应返回 nil")
	}
}

// ─── buildMessagesWithContext 测试 ─────────────────────────────────────────────

func TestBuildMessagesWithContext_Basic(t *testing.T) {
	history := []session.Message{
		{Role: "user", Content: "你好"},
		{Role: "ai", Content: "你好！有什么可以帮你的？"},
	}
	messages, evicted := buildMessagesWithContext(history, "", "", "", "", 0)
	if len(messages) != 3 { // system + 2 history
		t.Errorf("期望 3 条消息，实际 %d", len(messages))
	}
	if len(evicted) != 0 {
		t.Errorf("不应有被裁剪的消息，实际 %d", len(evicted))
	}
	if messages[0].Role != model.RoleSystem {
		t.Error("第一条消息应为 system")
	}
}

func TestBuildMessagesWithContext_WithSummary(t *testing.T) {
	history := []session.Message{
		{Role: "user", Content: "继续上次的话题"},
	}
	summary := "用户之前讨论了Go语言的并发模型"
	messages, _ := buildMessagesWithContext(history, "", "", summary, "", 0)
	// System Prompt 应包含摘要
	systemContent := messages[0].Content
	if !containsSubstring(systemContent, "对话历史摘要") {
		t.Error("System Prompt 应包含摘要标题")
	}
	if !containsSubstring(systemContent, summary) {
		t.Error("System Prompt 应包含摘要内容")
	}
}

func TestBuildMessagesWithContext_WithRAG(t *testing.T) {
	history := []session.Message{
		{Role: "user", Content: "什么是量子计算？"},
	}
	ragContext := "[1] 量子计算是利用量子力学原理进行计算的技术。"
	messages, _ := buildMessagesWithContext(history, "", ragContext, "", "", 0)
	systemContent := messages[0].Content
	if !containsSubstring(systemContent, "参考知识库") {
		t.Error("System Prompt 应包含 RAG 标题")
	}
	if !containsSubstring(systemContent, ragContext) {
		t.Error("System Prompt 应包含 RAG 内容")
	}
}

func TestBuildMessagesWithContext_WithTokenBudget(t *testing.T) {
	// 创建大量历史消息（每条消息较长，确保超出 token 预算）
	history := make([]session.Message, 100)
	longMsg := "这是一条非常长的测试消息，用于验证 token 预算裁剪功能。我们需要确保消息足够长，以便在有限的 token 预算下触发裁剪。这段文字会重复多次以增加长度。"
	for i := range history {
		if i%2 == 0 {
			history[i] = session.Message{Role: "user", Content: longMsg + longMsg + longMsg}
		} else {
			history[i] = session.Message{Role: "ai", Content: longMsg + longMsg + longMsg}
		}
	}
	messages, evicted := buildMessagesWithContext(history, "你是一个助手", "", "", "", 4096)
	if len(evicted) == 0 {
		t.Error("100 条消息在 4096 token 预算下应有裁剪")
	}
	// 消息列表第一条应为 system
	if messages[0].Role != model.RoleSystem {
		t.Error("第一条消息应为 system")
	}
	// 总消息数应为 system + kept
	if len(messages) != len(history)-len(evicted)+1 {
		t.Errorf("消息数不匹配：messages=%d, history=%d, evicted=%d", len(messages), len(history), len(evicted))
	}
}

func TestBuildMessagesWithContext_FallbackToMaxPromptMessages(t *testing.T) {
	// maxContextTokens=0 时应回退到固定消息数滑动窗口
	history := make([]session.Message, 30)
	for i := range history {
		history[i] = session.Message{Role: "user", Content: "消息"}
	}
	messages, evicted := buildMessagesWithContext(history, "", "", "", "", 0)
	// 应保留 maxPromptMessages=20 条
	if len(messages) != 21 { // system + 20
		t.Errorf("回退模式应保留 %d 条消息（system+20），实际 %d", maxPromptMessages+1, len(messages))
	}
	if len(evicted) != 10 {
		t.Errorf("应裁剪 10 条消息，实际 %d", len(evicted))
	}
}

// containsSubstring 检查字符串是否包含子串
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
