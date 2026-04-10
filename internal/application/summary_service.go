// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"fmt"
	"strings"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/session"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// ─── 会话摘要服务 ─────────────────────────────────────────────────────────────

// SummaryService 会话摘要服务
// 负责在对话超出 token 预算时，自动将旧消息压缩为摘要
// 采用增量摘要策略：将"旧摘要 + 被淘汰消息"合并生成新摘要
// 参考 LangChain ConversationSummaryBufferMemory
type SummaryService struct {
	sessionRepo  session.Repository
	modelGen     model.Generator     // 用于生成摘要的模型
	modelFactory func(string) model.Generator
	defaultModel string
}

// NewSummaryService 创建摘要服务
func NewSummaryService(sessionRepo session.Repository, modelGen model.Generator) *SummaryService {
	return &SummaryService{
		sessionRepo: sessionRepo,
		modelGen:    modelGen,
	}
}

// SetModelFactory 设置模型工厂（支持动态选择模型生成摘要）
func (s *SummaryService) SetModelFactory(factory func(string) model.Generator, defaultModel string) {
	s.modelFactory = factory
	s.defaultModel = defaultModel
}

// getModelGen 获取模型生成器
func (s *SummaryService) getModelGen(modelName string) model.Generator {
	if modelName == "" || s.modelFactory == nil {
		return s.modelGen
	}
	return s.modelFactory(modelName)
}

// summaryTriggerThreshold 触发摘要生成的消息数阈值
// 当会话消息数超过此值时，才考虑生成摘要
const summaryTriggerThreshold = 10

// ShouldGenerateSummary 判断是否需要生成/更新摘要
// 条件：会话消息数超过阈值，且有消息被 token 预算裁剪
func (s *SummaryService) ShouldGenerateSummary(ctx context.Context, sessID session.SessionID, evictedCount int) bool {
	if evictedCount == 0 {
		return false
	}
	msgCount, err := s.sessionRepo.GetSessionMessageCount(ctx, sessID)
	if err != nil {
		return false
	}
	return msgCount >= summaryTriggerThreshold
}

// GenerateIncrementalSummary 增量生成会话摘要
// 将"旧摘要 + 被淘汰的消息"合并，调用 LLM 生成新摘要
// 这样每次摘要不需要重新处理全部历史，只处理增量部分
func (s *SummaryService) GenerateIncrementalSummary(ctx context.Context, sessID session.SessionID, evictedMessages []session.Message, modelName string) {
	logger := shared.GetLogger()

	// 获取旧摘要
	oldSummary, err := s.sessionRepo.GetSessionSummary(ctx, sessID)
	if err != nil {
		logger.Warn("获取旧摘要失败", zap.String("session_id", string(sessID)), zap.Error(err))
		oldSummary = ""
	}

	// 构建被淘汰消息的文本
	var evictedText strings.Builder
	for _, msg := range evictedMessages {
		role := "用户"
		if msg.Role == "ai" {
			role = "AI"
		}
		evictedText.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	// 构建摘要生成 Prompt
	var prompt strings.Builder
	prompt.WriteString("请将以下对话信息压缩为简洁的摘要，保留关键信息、用户意图和重要结论。")
	prompt.WriteString("摘要应该是第三人称叙述，不超过 300 字。\n\n")

	if oldSummary != "" {
		prompt.WriteString("## 已有摘要\n")
		prompt.WriteString(oldSummary)
		prompt.WriteString("\n\n## 新增对话内容\n")
	} else {
		prompt.WriteString("## 对话内容\n")
	}
	prompt.WriteString(evictedText.String())
	prompt.WriteString("\n请生成更新后的完整摘要：")

	// 调用 LLM 生成摘要
	gen := s.getModelGen(modelName)
	result, err := gen.Generate(ctx, model.Prompt(prompt.String()))
	if err != nil {
		logger.Warn("生成会话摘要失败",
			zap.String("session_id", string(sessID)),
			zap.Error(err),
		)
		return
	}

	newSummary := strings.TrimSpace(string(result.Response))
	if newSummary == "" {
		return
	}

	// 持久化摘要
	if err := s.sessionRepo.UpdateSessionSummary(ctx, sessID, newSummary); err != nil {
		logger.Warn("保存会话摘要失败",
			zap.String("session_id", string(sessID)),
			zap.Error(err),
		)
		return
	}

	logger.Info("会话摘要已更新",
		zap.String("session_id", string(sessID)),
		zap.Int("evicted_msgs", len(evictedMessages)),
		zap.Int("summary_len", len(newSummary)),
	)
}

// GetSessionSummary 获取会话摘要
func (s *SummaryService) GetSessionSummary(ctx context.Context, sessID session.SessionID) (string, error) {
	return s.sessionRepo.GetSessionSummary(ctx, sessID)
}
