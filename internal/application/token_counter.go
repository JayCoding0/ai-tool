// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"unicode/utf8"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
)

// ─── Token 计数与上下文窗口管理 ─────────────────────────────────────────────

// TokenBudget 动态 token 预算分配
type TokenBudget struct {
	MaxContextTokens int // 模型最大上下文 token 数
	SystemPrompt     int // System Prompt 预留
	RAGContext       int // RAG 知识库检索结果预留
	Summary          int // 会话摘要预留
	ReplyReserve     int // 回复预留空间
	HistoryBudget    int // 历史消息可用 token 数（自动计算）
}

// DefaultMaxContextTokens 默认最大上下文 token 数（适用于大多数模型）
const DefaultMaxContextTokens = 8192

// ReplyReserveTokens 回复预留 token 数
const ReplyReserveTokens = 1500

// SummaryReserveTokens 摘要预留 token 数
const SummaryReserveTokens = 500

// EstimateTokens 估算文本的 token 数
// 采用混合估算策略：中文约 1.5 字/token，英文约 4 字符/token
// 这里使用简化公式：rune 数 * 2/3 作为近似值
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	runeCount := utf8.RuneCountInString(text)
	// 中英文混合场景的经验公式
	return max(1, runeCount*2/3)
}

// EstimateMessagesTokens 估算消息列表的总 token 数
// 每条消息额外计算 4 token 的消息格式开销（role 标记等）
func EstimateMessagesTokens(messages []model.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg.Content) + 4 // 4 token 消息格式开销
	}
	return total
}

// EstimateSessionMessagesTokens 估算会话消息列表的总 token 数
func EstimateSessionMessagesTokens(messages []session.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg.Content) + 4
	}
	return total
}

// CalculateTokenBudget 计算动态 token 预算
// maxContextTokens: 模型最大上下文 token 数（从配置获取）
// systemPrompt: 当前 System Prompt 文本
// ragContext: RAG 检索结果文本
// summary: 当前会话摘要文本
func CalculateTokenBudget(maxContextTokens int, systemPrompt, ragContext, summary string) TokenBudget {
	if maxContextTokens <= 0 {
		maxContextTokens = DefaultMaxContextTokens
	}

	budget := TokenBudget{
		MaxContextTokens: maxContextTokens,
		SystemPrompt:     EstimateTokens(systemPrompt) + 4,
		RAGContext:        EstimateTokens(ragContext),
		Summary:          EstimateTokens(summary),
		ReplyReserve:     ReplyReserveTokens,
	}

	// 历史消息可用 token = 总预算 - 各项预留
	budget.HistoryBudget = maxContextTokens - budget.SystemPrompt - budget.RAGContext - budget.Summary - budget.ReplyReserve
	if budget.HistoryBudget < 200 {
		budget.HistoryBudget = 200 // 至少保留 200 token 给历史消息
	}

	return budget
}

// TrimHistoryByTokenBudget 根据 token 预算裁剪历史消息
// 从最新消息开始保留，直到 token 预算用尽
// 返回裁剪后的消息列表和被裁剪掉的消息列表
func TrimHistoryByTokenBudget(history []session.Message, tokenBudget int) (kept []session.Message, evicted []session.Message) {
	if len(history) == 0 {
		return nil, nil
	}

	// 从后往前累加 token，找到能保留的起始位置
	totalTokens := 0
	startIdx := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(history[i].Content) + 4
		if totalTokens+msgTokens > tokenBudget {
			break
		}
		totalTokens += msgTokens
		startIdx = i
	}

	if startIdx > 0 {
		evicted = history[:startIdx]
	}
	kept = history[startIdx:]
	return kept, evicted
}

// max 返回两个整数中的较大值
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
