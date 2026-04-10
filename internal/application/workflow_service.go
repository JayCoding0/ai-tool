// Package application 应用服务层，编排领域对象完成业务用例
package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aiProject/internal/domain/workflow"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

// WorkflowService 工作流应用服务（CRUD + 发布管理）
type WorkflowService struct {
	workflowRepo workflow.Repository
	runRepo      workflow.RunRepository
}

// NewWorkflowService 创建工作流服务
func NewWorkflowService(repo workflow.Repository, runRepo workflow.RunRepository) *WorkflowService {
	return &WorkflowService{
		workflowRepo: repo,
		runRepo:      runRepo,
	}
}

// CreateWorkflow 创建工作流
func (s *WorkflowService) CreateWorkflow(ctx context.Context, wf *workflow.Workflow) error {
	// 校验工作流定义
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("工作流校验失败: %w", err)
	}

	if err := s.workflowRepo.Create(ctx, wf); err != nil {
		return err
	}

	shared.GetLogger().Info("工作流创建成功",
		zap.Int64("id", wf.ID),
		zap.String("name", wf.Name),
		zap.Int64("user_id", wf.UserID),
	)
	return nil
}

// UpdateWorkflow 更新工作流（保存画布）
func (s *WorkflowService) UpdateWorkflow(ctx context.Context, wf *workflow.Workflow) error {
	// 校验工作流定义
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("工作流校验失败: %w", err)
	}

	// 检查工作流是否存在
	existing, err := s.workflowRepo.GetByID(ctx, wf.ID)
	if err != nil {
		return err
	}

	// 已发布的工作流不能直接修改，需要先改为草稿
	if existing.Status == workflow.StatusPublished && wf.Status != workflow.StatusDraft {
		return fmt.Errorf("已发布的工作流不能直接修改，请先改为草稿状态")
	}

	// 版本号自增
	wf.Version = existing.Version + 1

	return s.workflowRepo.Update(ctx, wf)
}

// GetWorkflow 获取工作流详情
func (s *WorkflowService) GetWorkflow(ctx context.Context, id int64) (*workflow.Workflow, error) {
	return s.workflowRepo.GetByID(ctx, id)
}

// ListWorkflows 列出用户的工作流
func (s *WorkflowService) ListWorkflows(ctx context.Context, userID int64, status workflow.Status) ([]*workflow.Workflow, error) {
	return s.workflowRepo.List(ctx, userID, status)
}

// DeleteWorkflow 删除工作流
func (s *WorkflowService) DeleteWorkflow(ctx context.Context, id int64) error {
	return s.workflowRepo.Delete(ctx, id)
}

// PublishWorkflow 发布工作流（draft → published）
func (s *WorkflowService) PublishWorkflow(ctx context.Context, id int64) error {
	wf, err := s.workflowRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if wf.Status == workflow.StatusPublished {
		return fmt.Errorf("工作流已经是发布状态")
	}

	// 发布前再次校验
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("工作流校验失败，无法发布: %w", err)
	}

	return s.workflowRepo.UpdateStatus(ctx, id, workflow.StatusPublished)
}

// ArchiveWorkflow 归档工作流
func (s *WorkflowService) ArchiveWorkflow(ctx context.Context, id int64) error {
	return s.workflowRepo.UpdateStatus(ctx, id, workflow.StatusArchived)
}

// GetWorkflowRuns 获取工作流执行记录
func (s *WorkflowService) GetWorkflowRuns(ctx context.Context, workflowID int64, limit int) ([]*workflow.WorkflowRun, error) {
	return s.runRepo.ListByWorkflowID(ctx, workflowID, limit)
}

// GetWorkflowRun 获取单次执行详情
func (s *WorkflowService) GetWorkflowRun(ctx context.Context, runID string) (*workflow.WorkflowRun, error) {
	return s.runRepo.GetByRunID(ctx, runID)
}

// ─── 导入/导出（Phase 3）──────────────────────────────────────────────────────

// WorkflowExportData 工作流导出数据格式
type WorkflowExportData struct {
	FormatVersion string              `json:"format_version"` // 导出格式版本
	ExportedAt    string              `json:"exported_at"`    // 导出时间
	Workflow      WorkflowExportInfo  `json:"workflow"`       // 工作流信息
}

// WorkflowExportInfo 工作流导出信息
type WorkflowExportInfo struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Nodes       []workflow.Node     `json:"nodes"`
	Edges       []workflow.Edge     `json:"edges"`
	Variables   []workflow.Variable `json:"variables,omitempty"`
}

// ExportWorkflow 导出工作流为 JSON 格式
func (s *WorkflowService) ExportWorkflow(ctx context.Context, id int64) (*WorkflowExportData, error) {
	wf, err := s.workflowRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("获取工作流失败: %w", err)
	}

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

	shared.GetLogger().Info("工作流导出成功",
		zap.Int64("id", id),
		zap.String("name", wf.Name),
	)

	return exportData, nil
}

// ImportWorkflow 从 JSON 数据导入工作流
func (s *WorkflowService) ImportWorkflow(ctx context.Context, data []byte, userID int64) (*workflow.Workflow, error) {
	var exportData WorkflowExportData
	if err := json.Unmarshal(data, &exportData); err != nil {
		return nil, fmt.Errorf("解析导入数据失败: %w", err)
	}

	if exportData.Workflow.Name == "" {
		return nil, fmt.Errorf("导入数据中缺少工作流名称")
	}

	// 创建新的工作流（导入时自动添加 "[导入]" 前缀避免重名）
	wf := &workflow.Workflow{
		Name:        exportData.Workflow.Name + " [导入]",
		Description: exportData.Workflow.Description,
		Nodes:       exportData.Workflow.Nodes,
		Edges:       exportData.Workflow.Edges,
		Variables:   exportData.Workflow.Variables,
		Status:      workflow.StatusDraft,
		UserID:      userID,
	}

	// 校验工作流定义
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("导入的工作流校验失败: %w", err)
	}

	if err := s.workflowRepo.Create(ctx, wf); err != nil {
		return nil, fmt.Errorf("保存导入的工作流失败: %w", err)
	}

	shared.GetLogger().Info("工作流导入成功",
		zap.Int64("id", wf.ID),
		zap.String("name", wf.Name),
		zap.Int64("user_id", userID),
	)

	return wf, nil
}
