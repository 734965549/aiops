// Package persistence 提供 Identity 用户仓储的 GORM 实现。
package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

// userModel 是数据库映射结构，仅在 persistence 内部使用，不暴露到 domain。
//
// 遵循平台建表规范：
//   - 嵌入 database.BaseModel 提供自增主键 id 与 created_at/updated_at；
//   - 业务标识 UserID 单独成列（user_id），承担跨上下文 FK 引用；
//   - 时间戳由 BaseModel 的 GORM 钩子在程序内设置，不依赖 DB DEFAULT。
type userModel struct {
	database.BaseModel
	UserID       string `gorm:"column:user_id;type:varchar(36);uniqueIndex;not null"`
	Username     string `gorm:"column:username;type:varchar(64);uniqueIndex;not null"`
	DisplayName  string `gorm:"column:display_name;type:varchar(128);not null;default:''"`
	Email        string `gorm:"column:email;type:varchar(128);not null;default:'';index"`
	PasswordHash string `gorm:"column:password_hash;type:varchar(255);not null;default:''"`
	Status       string `gorm:"column:status;type:varchar(32);not null;default:'active';index"`
}

// TableName 显式声明表名，避免 GORM 自动复数化造成与 SQL 迁移文件不一致。
func (userModel) TableName() string { return "iam_user" }

// UserRepository 基于 GORM 的实现。
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 构造仓储实例。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByID 按业务 user_id 查询；找不到返回 (nil, nil) 而不是 error，
// 让上层根据语义自行决定是 NOT_FOUND 还是其它分支。
//
// 入参 id 是业务标识（UUID），而不是 DB 自增主键。
func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	var m userModel
	if err := r.db.WithContext(ctx).First(&m, "user_id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomain(&m), nil
}

// FindByUsername 按用户名查询，语义同 FindByID。
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, nil
	}
	var m userModel
	if err := r.db.WithContext(ctx).First(&m, "username = ?", username).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomain(&m), nil
}

// Create 插入新用户。冲突时由调用方根据 PG 唯一约束错误自行处理。
//
// 不在这里做"用户已存在"的预查 + INSERT，避免双查询竞态：
// 高并发下应靠数据库唯一约束兜底，application 层再把 UNIQUE_VIOLATION 翻译为业务错误。
func (r *UserRepository) List(ctx context.Context, filter domain.UserFilter) ([]domain.User, error) {
	var rows []userModel
	q := applyUserFilter(r.db.WithContext(ctx).Model(&userModel{}), filter)
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.User, 0, len(rows))
	for i := range rows {
		out = append(out, *toDomain(&rows[i]))
	}
	return out, nil
}

func (r *UserRepository) Count(ctx context.Context, filter domain.UserFilter) (int64, error) {
	q := applyUserFilter(r.db.WithContext(ctx).Model(&userModel{}), filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func applyUserFilter(q *gorm.DB, filter domain.UserFilter) *gorm.DB {
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}
	keyword := strings.TrimSpace(filter.Keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("username ILIKE ? OR display_name ILIKE ? OR email ILIKE ?", like, like, like)
	}
	return q
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	if u == nil {
		return errors.New("user is nil")
	}
	u.ID = strings.TrimSpace(u.ID)
	u.Username = strings.TrimSpace(u.Username)
	m := userModel{
		UserID:       u.ID,
		Username:     u.Username,
		DisplayName:  u.DisplayName,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Status:       string(u.Status),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	u.CreatedAt = m.CreatedAt
	u.UpdatedAt = m.UpdatedAt
	return nil
}

// Update 更新用户非敏感资料与状态；不修改 username / password_hash。
func (r *UserRepository) Update(ctx context.Context, u *domain.User) error {
	if u == nil || strings.TrimSpace(u.ID) == "" {
		return errors.New("user is nil or missing id")
	}
	updates := map[string]any{
		"display_name": u.DisplayName,
		"email":        u.Email,
		"status":       string(u.Status),
	}
	res := r.db.WithContext(ctx).Model(&userModel{}).Where("user_id = ?", u.ID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Reactivate 把指定用户重置为 active 并写入新的密码哈希。
//
// 仅服务于 bootstrap 重新激活被迁移 0044 锁定的默认管理员：Update 故意不修改
// password_hash，这里通过独立的受控写路径一次性更新 password_hash 与 status，
// 单条 UPDATE 原子完成，updated_at 由 GORM Hook 维护。找不到记录返回
// gorm.ErrRecordNotFound，与 Update 语义一致。
func (r *UserRepository) Reactivate(ctx context.Context, userID, passwordHash string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errors.New("user id is required")
	}
	if passwordHash == "" {
		return errors.New("password hash is required")
	}
	res := r.db.WithContext(ctx).Model(&userModel{}).Where("user_id = ?", userID).
		Updates(map[string]any{
			"password_hash": passwordHash,
			"status":        string(domain.UserStatusActive),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteByID 删除平台用户（用于预置失败时的补偿回滚）。
func (r *UserRepository) DeleteByID(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("user id is required")
	}
	res := r.db.WithContext(ctx).Where("user_id = ?", id).Delete(&userModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// toDomain 把持久化模型转换为领域模型；domain.User.ID 对应业务标识 user_id，
// 数据库自增主键不暴露到 domain / application / interfaces 层。
func toDomain(m *userModel) *domain.User {
	if m == nil {
		return nil
	}
	return &domain.User{
		ID:           m.UserID,
		Username:     m.Username,
		DisplayName:  m.DisplayName,
		Email:        m.Email,
		PasswordHash: m.PasswordHash,
		Status:       domain.UserStatus(m.Status),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
