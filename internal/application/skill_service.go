package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"aiProject/internal/domain/model"
	"aiProject/internal/domain/skill"
)

// SkillService 技能应用服务
type SkillService struct {
	skillRepo    skill.Repository
	modelFactory func(modelName string) model.Generator
	defaultModel string
}

// NewSkillService 创建技能服务
func NewSkillService(skillRepo skill.Repository) *SkillService {
	return &SkillService{skillRepo: skillRepo}
}

// SetModelFactory 注入模型工厂（用于 AI 生成 Skill）
func (s *SkillService) SetModelFactory(factory func(string) model.Generator, defaultModel string) {
	s.modelFactory = factory
	s.defaultModel = defaultModel
}

// CreateSkillRequest 创建技能请求
type CreateSkillRequest struct {
	UserID       int64
	Name         string
	Description  string
	Icon         string
	Pattern      skill.SkillPattern
	SystemPrompt string
	Tools        []string
	IsPublic     bool
}

// UpdateSkillRequest 更新技能请求
type UpdateSkillRequest struct {
	ID           skill.SkillID
	UserID       int64
	IsAdmin      bool // 是否为管理员（可修改任意技能）
	Name         string
	Description  string
	Icon         string
	Pattern      skill.SkillPattern
	SystemPrompt string
	Tools        []string
	IsPublic     bool
}

// CreateSkill 创建技能
func (s *SkillService) CreateSkill(ctx context.Context, req CreateSkillRequest) (*skill.Skill, error) {
	if req.Name == "" {
		return nil, errors.New("技能名称不能为空")
	}
	if req.SystemPrompt == "" {
		return nil, errors.New("System Prompt 不能为空")
	}
	if req.Icon == "" {
		req.Icon = "🤖"
	}

	sk := &skill.Skill{
		UserID:       req.UserID,
		Name:         req.Name,
		Description:  req.Description,
		Icon:         req.Icon,
		Pattern:      req.Pattern,
		SystemPrompt: req.SystemPrompt,
		Tools:        req.Tools,
		IsPublic:     req.IsPublic,
	}
	if err := s.skillRepo.Create(ctx, sk); err != nil {
		return nil, err
	}
	return sk, nil
}

// UpdateSkill 更新技能（admin 可修改任意技能，普通用户只能修改自己的）
func (s *SkillService) UpdateSkill(ctx context.Context, req UpdateSkillRequest) (*skill.Skill, error) {
	if req.Name == "" {
		return nil, errors.New("技能名称不能为空")
	}
	if req.SystemPrompt == "" {
		return nil, errors.New("System Prompt 不能为空")
	}

	sk, err := s.skillRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if sk == nil {
		return nil, errors.New("技能不存在")
	}
	// admin 可修改任意技能；普通用户只能修改自己的（不能修改预设和他人的）
	if !req.IsAdmin && (sk.IsSystem() || !sk.IsOwnedBy(req.UserID)) {
		return nil, errors.New("无权修改此技能")
	}

	sk.Name = req.Name
	sk.Description = req.Description
	sk.Icon = req.Icon
	sk.Pattern = req.Pattern
	sk.SystemPrompt = req.SystemPrompt
	sk.Tools = req.Tools
	sk.IsPublic = req.IsPublic

	if err := s.skillRepo.Update(ctx, sk); err != nil {
		return nil, err
	}
	return sk, nil
}

// DeleteSkill 删除技能（admin 可删除任意技能，普通用户只能删除自己的）
func (s *SkillService) DeleteSkill(ctx context.Context, id skill.SkillID, userID int64, isAdmin bool) error {
	sk, err := s.skillRepo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if sk == nil {
		return errors.New("技能不存在")
	}
	if !isAdmin && (sk.IsSystem() || !sk.IsOwnedBy(userID)) {
		return errors.New("无权删除此技能")
	}
	return s.skillRepo.Delete(ctx, id)
}

// ListSkills 列出用户可见的技能（admin 返回全部）
func (s *SkillService) ListSkills(ctx context.Context, userID int64, isAdmin bool) ([]*skill.Skill, error) {
	if isAdmin {
		return s.skillRepo.ListAll(ctx)
	}
	return s.skillRepo.ListByUser(ctx, userID)
}

// GetSkill 获取技能详情
func (s *SkillService) GetSkill(ctx context.Context, id skill.SkillID) (*skill.Skill, error) {
	return s.skillRepo.FindByID(ctx, id)
}

// SyncSkillsFromDirectory 从项目 skills/ 目录同步内置 Skill 到数据库
func (s *SkillService) SyncSkillsFromDirectory(ctx context.Context, skillsDir string) error {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	existing, _ := s.skillRepo.ListSystem(ctx)
	existingMap := make(map[string]*skill.Skill)
	for _, sk := range existing {
		existingMap[sk.Name] = sk
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(skillsDir, entry.Name())
		sk, err := skill.LoadFromDirectory(dirPath)
		if err != nil {
			continue
		}

		if old, exists := existingMap[sk.Name]; exists {
			sk.ID = old.ID
			sk.UserID = old.UserID
			_ = s.skillRepo.Update(ctx, sk)
		} else {
			_ = s.skillRepo.Create(ctx, sk)
		}
	}
	return nil
}