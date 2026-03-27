package application

import (
	"context"
	"strings"

	"aiProject/internal/domain/a2a"
	"aiProject/internal/domain/session"
	infra_a2a "aiProject/internal/infrastructure/a2a"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// A2AService A2A 任务调度服务
// 负责接收外部任务、驱动 ChatService 执行、通过 StreamHub 推送进度
type A2AService struct {
	chatService *ChatService
	taskStore   *infra_a2a.TaskStore
	streamHub   *infra_a2a.StreamHub
	agentCard   *a2a.AgentCard
}

// NewA2AService 创建 A2A 服务
func NewA2AService(chatService *ChatService, agentCard *a2a.AgentCard) *A2AService {
	return &A2AService{
		chatService: chatService,
		taskStore:   infra_a2a.NewTaskStore(),
		streamHub:   infra_a2a.NewStreamHub(),
		agentCard:   agentCard,
	}
}

// GetAgentCard 获取 AgentCard
func (s *A2AService) GetAgentCard() *a2a.AgentCard {
	return s.agentCard
}

// SubmitTask 提交任务（异步执行，立即返回任务对象）
func (s *A2AService) SubmitTask(ctx context.Context, req a2a.TaskSendRequest) (*a2a.Task, error) {
	// 生成任务 ID
	taskID := req.ID
	if taskID == "" {
		taskID = infra_a2a.GenerateTaskID()
	}

	// 创建任务并保存
	task := a2a.NewTask(taskID, req)
	s.taskStore.Save(task)

	// 异步执行任务（不阻塞当前请求）
	go s.executeTask(context.Background(), task)

	return task, nil
}

// GetTask 查询任务状态
func (s *A2AService) GetTask(taskID string) (*a2a.Task, bool) {
	return s.taskStore.Get(taskID)
}

// SubscribeTask 订阅任务流式事件
func (s *A2AService) SubscribeTask(taskID string) (<-chan a2a.TaskStreamEvent, func()) {
	return s.streamHub.Subscribe(taskID)
}

// executeTask 异步执行任务核心逻辑
func (s *A2AService) executeTask(ctx context.Context, task *a2a.Task) {
	logger := shared.GetLogger()
	taskID := task.ID

	// 更新状态为 working
	task.Transition(a2a.TaskStateWorking, "任务处理中")
	s.taskStore.Save(task)
	s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
		Type:   "status_update",
		TaskID: taskID,
		Status: &task.Status,
	})

	// 提取用户输入文本
	userText := extractTextFromParts(task.Input.Parts)
	if userText == "" {
		s.failTask(task, "输入消息为空")
		return
	}

	// 提取启用的工具列表（从 metadata 中读取）
	var enabledTools []string
	if task.Metadata != nil {
		if tools, ok := task.Metadata["enabled_tools"]; ok {
			if toolList, ok := tools.([]interface{}); ok {
				for _, t := range toolList {
					if name, ok := t.(string); ok {
						enabledTools = append(enabledTools, name)
					}
				}
			}
		}
	}

	// 构建 ChatRequest，复用现有 ChatService
	chatReq := ChatRequest{
		Message:   userText,
		SessionID: sessionIDFromA2A(task.SessionID),
		ModelName: extractStringMeta(task.Metadata, "model_name"),
	}

	// 选择处理路径：有工具 → ProcessMessageWithTools，无工具 → ProcessMessageStream
	var streamCh <-chan StreamChatResponse
	var err error
	if len(enabledTools) > 0 {
		streamCh, err = s.chatService.ProcessMessageWithTools(ctx, chatReq, enabledTools)
	} else {
		streamCh, err = s.chatService.ProcessMessageStream(ctx, chatReq)
	}
	if err != nil {
		logger.Error("A2A 任务启动失败", zap.String("task_id", taskID), zap.Error(err))
		s.failTask(task, "任务启动失败: "+err.Error())
		return
	}

	// 消费流式事件并转发给订阅者
	var fullContent strings.Builder
	for event := range streamCh {
		switch event.Type {
		case "chunk":
			if event.Content != "" {
				fullContent.WriteString(event.Content)
				s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
					Type:   "message_chunk",
					TaskID: taskID,
					Delta:  event.Content,
				})
			}
			if event.Thinking != "" {
				s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
					Type:     "message_chunk",
					TaskID:   taskID,
					Thinking: event.Thinking,
				})
			}
		case "thought":
			s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
				Type:   "message_chunk",
				TaskID: taskID,
				Delta:  event.Content,
				Step:   event.Step,
			})
		case "tool_call":
			s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
				Type:            "tool_call",
				TaskID:          taskID,
				ToolName:        event.ToolName,
				ToolDisplayName: event.ToolDisplayName,
				ToolCallID:      event.ToolCallID,
				ToolArgs:        event.ToolArgs,
				Step:            event.Step,
			})
		case "tool_result":
			s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
				Type:            "tool_result",
				TaskID:          taskID,
				ToolName:        event.ToolName,
				ToolDisplayName: event.ToolDisplayName,
				ToolCallID:      event.ToolCallID,
				ToolArgs:        event.ToolArgs,
				ToolResult:      event.ToolResult,
				Step:            event.Step,
			})
		case "error":
			s.failTask(task, event.Error)
			return
		case "done":
			// 任务完成
		}
	}

	// 构建最终输出消息
	finalText := fullContent.String()
	if finalText == "" {
		finalText = "（任务已完成，但未生成文本回复）"
	}
	task.AppendOutput(a2a.TaskMessage{
		Role: "agent",
		Parts: []a2a.TaskPart{
			{Type: "text", Text: finalText},
		},
	})
	task.Transition(a2a.TaskStateCompleted, "任务已完成")
	s.taskStore.Save(task)

	// 推送完成事件
	s.streamHub.Publish(taskID, a2a.TaskStreamEvent{
		Type:   "completed",
		TaskID: taskID,
		Status: &task.Status,
		Task:   task,
	})

	// 关闭所有订阅者
	s.streamHub.Close(taskID)
	logger.Info("A2A 任务完成", zap.String("task_id", taskID))
}

// failTask 将任务标记为失败并通知订阅者
func (s *A2AService) failTask(task *a2a.Task, reason string) {
	task.Transition(a2a.TaskStateFailed, reason)
	s.taskStore.Save(task)
	s.streamHub.Publish(task.ID, a2a.TaskStreamEvent{
		Type:   "error",
		TaskID: task.ID,
		Error:  reason,
		Status: &task.Status,
	})
	s.streamHub.Close(task.ID)
	shared.GetLogger().Error("A2A 任务失败",
		zap.String("task_id", task.ID),
		zap.String("reason", reason),
	)
}

// extractTextFromParts 从消息片段中提取纯文本内容
func extractTextFromParts(parts []a2a.TaskPart) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// sessionIDFromA2A 将 A2A 的 sessionID 转换为内部 SessionID 类型
func sessionIDFromA2A(id string) session.SessionID {
	return session.SessionID(id)
}

// extractStringMeta 从 metadata 中安全提取字符串值
func extractStringMeta(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
