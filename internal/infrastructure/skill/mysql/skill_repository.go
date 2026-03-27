package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"aiProject/internal/domain/skill"
	"aiProject/internal/infrastructure/database"
	trmysql "git.code.oa.com/trpc-go/trpc-database/mysql"
)

// SkillRepository MySQL 技能存储库
type SkillRepository struct {
	db trmysql.Client
}

// NewSkillRepository 创建 MySQL 技能存储库
func NewSkillRepository() *SkillRepository {
	return &SkillRepository{
		db: database.GetDB(),
	}
}

// Create 创建技能
func (r *SkillRepository) Create(ctx context.Context, s *skill.Skill) error {
	isPublic := 0
	if s.IsPublic {
		isPublic = 1
	}
	toolsJSON := marshalTools(s.Tools)
	result, err := r.db.Exec(ctx,
		"INSERT INTO skills (user_id, name, description, icon, system_prompt, pattern, tools, is_public) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		s.UserID, s.Name, s.Description, s.Icon, s.SystemPrompt, string(s.Pattern), toolsJSON, isPublic)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = skill.SkillID(id)
	return nil
}

// Update 更新技能
func (r *SkillRepository) Update(ctx context.Context, s *skill.Skill) error {
	isPublic := 0
	if s.IsPublic {
		isPublic = 1
	}
	toolsJSON := marshalTools(s.Tools)
	_, err := r.db.Exec(ctx,
		"UPDATE skills SET name=?, description=?, icon=?, system_prompt=?, pattern=?, tools=?, is_public=? WHERE id=?",
		s.Name, s.Description, s.Icon, s.SystemPrompt, string(s.Pattern), toolsJSON, isPublic, int64(s.ID))
	return err
}

// Delete 删除技能
func (r *SkillRepository) Delete(ctx context.Context, id skill.SkillID) error {
	_, err := r.db.Exec(ctx, "DELETE FROM skills WHERE id=?", int64(id))
	return err
}

// FindByID 根据ID查找技能
func (r *SkillRepository) FindByID(ctx context.Context, id skill.SkillID) (*skill.Skill, error) {
	var s skill.Skill
	var isPublic int
	var pattern string
	var toolsJSON sql.NullString
	var createdAt, updatedAt time.Time
	dest := []interface{}{
		&s.ID, &s.UserID, &s.Name, &s.Description, &s.Icon,
		&s.SystemPrompt, &pattern, &toolsJSON, &isPublic, &createdAt, &updatedAt,
	}
	err := r.db.QueryRow(ctx, dest,
		"SELECT id, user_id, name, description, icon, system_prompt, COALESCE(pattern,''), COALESCE(tools,'[]'), is_public, created_at, updated_at FROM skills WHERE id=?",
		int64(id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.IsPublic = isPublic == 1
	s.Pattern = skill.SkillPattern(pattern)
	s.Tools = unmarshalTools(toolsJSON.String)
	s.CreatedAt = createdAt
	s.UpdatedAt = updatedAt
	return &s, nil
}

// ListByUser 列出用户可见的技能（系统预设 + 公开 + 自己的）
func (r *SkillRepository) ListByUser(ctx context.Context, userID int64) ([]*skill.Skill, error) {
	query := `
		SELECT id, user_id, name, description, icon, system_prompt, COALESCE(pattern,''), COALESCE(tools,'[]'), is_public, created_at, updated_at
		FROM skills
		WHERE user_id = 0 OR is_public = 1 OR user_id = ?
		ORDER BY user_id ASC, id ASC
	`
	return r.querySkills(ctx, query, userID)
}

// ListAll 列出所有技能（admin 专用）
func (r *SkillRepository) ListAll(ctx context.Context) ([]*skill.Skill, error) {
	query := `
		SELECT id, user_id, name, description, icon, system_prompt, COALESCE(pattern,''), COALESCE(tools,'[]'), is_public, created_at, updated_at
		FROM skills
		ORDER BY user_id ASC, id ASC
	`
	return r.querySkills(ctx, query)
}

// ListSystem 列出所有系统预设技能
func (r *SkillRepository) ListSystem(ctx context.Context) ([]*skill.Skill, error) {
	query := `
		SELECT id, user_id, name, description, icon, system_prompt, COALESCE(pattern,''), COALESCE(tools,'[]'), is_public, created_at, updated_at
		FROM skills
		WHERE user_id = 0
		ORDER BY id ASC
	`
	return r.querySkills(ctx, query)
}

// querySkills 通用查询技能列表
func (r *SkillRepository) querySkills(ctx context.Context, query string, args ...interface{}) ([]*skill.Skill, error) {
	var skills []*skill.Skill
	err := r.db.Query(ctx, func(rows *sql.Rows) error {
		var s skill.Skill
		var isPublic int
		var pattern string
		var toolsJSON sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.Name, &s.Description, &s.Icon,
			&s.SystemPrompt, &pattern, &toolsJSON, &isPublic, &createdAt, &updatedAt,
		); err != nil {
			return err
		}
		s.IsPublic = isPublic == 1
		s.Pattern = skill.SkillPattern(pattern)
		s.Tools = unmarshalTools(toolsJSON.String)
		s.CreatedAt = createdAt
		s.UpdatedAt = updatedAt
		skills = append(skills, &s)
		return nil
	}, query, args...)
	if err != nil {
		return nil, err
	}
	return skills, nil
}

// marshalTools 将工具列表序列化为 JSON 字符串
func marshalTools(tools []string) string {
	if len(tools) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(tools)
	return string(b)
}

// unmarshalTools 将 JSON 字符串反序列化为工具列表
func unmarshalTools(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var tools []string
	json.Unmarshal([]byte(s), &tools) //nolint:errcheck
	return tools
}
