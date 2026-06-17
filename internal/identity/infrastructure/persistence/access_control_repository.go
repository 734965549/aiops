// Package persistence 提供 Identity 权限域的 GORM 实现。
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/identity/domain"
	"github.com/734965549/aiops/pkg/database"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type roleModel struct {
	database.BaseModel
	RoleID      string `gorm:"column:role_id;type:varchar(36);uniqueIndex;not null"`
	Code        string `gorm:"column:code;type:varchar(64);uniqueIndex;not null"`
	Name        string `gorm:"column:name;type:varchar(128);not null"`
	Description string `gorm:"column:description;type:varchar(255);not null;default:''"`
	Status      string `gorm:"column:status;type:varchar(32);not null;default:'active';index"`
	IsSystem    bool   `gorm:"column:is_system;not null;default:false"`
}

func (roleModel) TableName() string { return "iam_role" }

type permissionModel struct {
	database.BaseModel
	PermissionID string `gorm:"column:permission_id;type:varchar(36);uniqueIndex;not null"`
	Code         string `gorm:"column:code;type:varchar(128);uniqueIndex;not null"`
	Name         string `gorm:"column:name;type:varchar(128);not null"`
	Resource     string `gorm:"column:resource;type:varchar(128);index;not null"`
	Action       string `gorm:"column:action;type:varchar(64);index;not null"`
	Description  string `gorm:"column:description;type:varchar(255);not null;default:''"`
}

func (permissionModel) TableName() string { return "iam_permission" }

type userRoleModel struct {
	database.BaseModel
	UserID string `gorm:"column:user_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_user_role"`
	RoleID string `gorm:"column:role_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_user_role"`
	Source string `gorm:"column:source;type:varchar(32);not null;default:manual;index:idx_iam_user_role_user_source,priority:2"`
}

func (userRoleModel) TableName() string { return "iam_user_role" }

type rolePermissionModel struct {
	database.BaseModel
	RoleID       string `gorm:"column:role_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_role_permission"`
	PermissionID string `gorm:"column:permission_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_role_permission"`
}

func (rolePermissionModel) TableName() string { return "iam_role_permission" }

type dataScopeModel struct {
	database.BaseModel
	DataScopeID string          `gorm:"column:data_scope_id;type:varchar(36);uniqueIndex;not null"`
	Code        string          `gorm:"column:code;type:varchar(64);uniqueIndex;not null"`
	Name        string          `gorm:"column:name;type:varchar(128);not null"`
	ScopeType   string          `gorm:"column:scope_type;type:varchar(32);index;not null"`
	ScopeConfig json.RawMessage `gorm:"column:scope_config;type:jsonb;not null;default:'{}'::jsonb"`
	Description string          `gorm:"column:description;type:varchar(255);not null;default:''"`
}

func (dataScopeModel) TableName() string { return "iam_data_scope" }

type roleDataScopeModel struct {
	database.BaseModel
	RoleID      string `gorm:"column:role_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_role_data_scope"`
	DataScopeID string `gorm:"column:data_scope_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_role_data_scope"`
}

func (roleDataScopeModel) TableName() string { return "iam_role_data_scope" }

type aiToolPermissionModel struct {
	database.BaseModel
	ToolPermissionID string `gorm:"column:tool_permission_id;type:varchar(36);uniqueIndex;not null"`
	ToolCode         string `gorm:"column:tool_code;type:varchar(128);uniqueIndex;not null"`
	ToolName         string `gorm:"column:tool_name;type:varchar(128);not null"`
	PermissionMode   string `gorm:"column:permission_mode;type:varchar(32);index;not null"`
	AllowConfirm     bool   `gorm:"column:allow_confirm;not null;default:false"` // DB 列 allow_confirm 映射 PermitsUnconfirmedInvoke
	Description      string `gorm:"column:description;type:varchar(255);not null;default:''"`
}

func (aiToolPermissionModel) TableName() string { return "iam_ai_tool_permission" }

type roleAIToolPermissionModel struct {
	database.BaseModel
	RoleID           string `gorm:"column:role_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_role_ai_tool_permission"`
	ToolPermissionID string `gorm:"column:tool_permission_id;type:varchar(36);index;not null;uniqueIndex:uq_iam_role_ai_tool_permission"`
}

func (roleAIToolPermissionModel) TableName() string { return "iam_role_ai_tool_permission" }

// AccessControlRepository 基于 GORM 的 Identity 权限域查询实现。
type AccessControlRepository struct {
	db                   *gorm.DB
	grantCache           userGrantCache
	loadUserGrantContext func(context.Context, string) (*domain.UserGrantContext, error) // 仅测试注入
}

// NewAccessControlRepository 构造权限仓储。redis 非空且 grantCacheTTL > 0 时为 LoadUserGrantContext 启用 Redis 短期缓存。
func NewAccessControlRepository(db *gorm.DB, redisClient *redis.Client, grantCacheTTL time.Duration) *AccessControlRepository {
	return &AccessControlRepository{
		db:         db,
		grantCache: newRedisUserGrantCache(redisClient, grantCacheTTL),
	}
}

func (r *AccessControlRepository) ListRoles(ctx context.Context, filter domain.RoleFilter) ([]domain.Role, error) {
	var rows []roleModel
	q := r.db.WithContext(ctx).Model(&roleModel{})
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}
	if filter.IsSystem != nil {
		q = q.Where("is_system = ?", *filter.IsSystem)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return mapRoles(rows), nil
}

func (r *AccessControlRepository) CountRoles(ctx context.Context, filter domain.RoleFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&roleModel{})
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}
	if filter.IsSystem != nil {
		q = q.Where("is_system = ?", *filter.IsSystem)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *AccessControlRepository) FindRoleByID(ctx context.Context, id string) (*domain.Role, error) {
	return r.findRole(ctx, "role_id = ?", id)
}
func (r *AccessControlRepository) FindRoleByCode(ctx context.Context, code string) (*domain.Role, error) {
	return r.findRole(ctx, "code = ?", code)
}

func (r *AccessControlRepository) findRole(ctx context.Context, cond string, arg any) (*domain.Role, error) {
	var row roleModel
	if err := r.db.WithContext(ctx).First(&row, cond, arg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	m := toDomainRole(&row)
	return m, nil
}

func (r *AccessControlRepository) ListPermissions(ctx context.Context, filter domain.PermissionFilter) ([]domain.Permission, error) {
	var rows []permissionModel
	q := r.db.WithContext(ctx).Model(&permissionModel{})
	if filter.Resource != "" {
		q = q.Where("resource = ?", filter.Resource)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return mapPermissions(rows), nil
}

func (r *AccessControlRepository) CountPermissions(ctx context.Context, filter domain.PermissionFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&permissionModel{})
	if filter.Resource != "" {
		q = q.Where("resource = ?", filter.Resource)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *AccessControlRepository) FindPermissionByID(ctx context.Context, id string) (*domain.Permission, error) {
	return r.findPermission(ctx, "permission_id = ?", id)
}
func (r *AccessControlRepository) FindPermissionByCode(ctx context.Context, code string) (*domain.Permission, error) {
	return r.findPermission(ctx, "code = ?", code)
}

func (r *AccessControlRepository) findPermission(ctx context.Context, cond string, arg any) (*domain.Permission, error) {
	var row permissionModel
	if err := r.db.WithContext(ctx).First(&row, cond, arg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	m := toDomainPermission(&row)
	return m, nil
}

func (r *AccessControlRepository) ListUserRoles(ctx context.Context, userID string) ([]domain.Role, error) {
	var rows []roleModel
	if err := r.db.WithContext(ctx).
		Table("iam_role").
		Select("iam_role.*").
		Joins("JOIN iam_user_role ON iam_user_role.role_id = iam_role.role_id").
		Where("iam_user_role.user_id = ?", strings.TrimSpace(userID)).
		Order("iam_role.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapRoles(rows), nil
}

func (r *AccessControlRepository) ListUserRoleBindings(ctx context.Context, userID string) ([]domain.UserRole, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	var rows []userRoleModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.UserRole, 0, len(rows))
	for i := range rows {
		out = append(out, toDomainUserRole(&rows[i]))
	}
	return out, nil
}

func (r *AccessControlRepository) BindUserRole(ctx context.Context, userID, roleID string, source domain.UserRoleSource) error {
	userID = strings.TrimSpace(userID)
	roleID = strings.TrimSpace(roleID)
	if userID == "" || roleID == "" {
		return fmt.Errorf("userID and roleID are required")
	}
	source = domain.NormalizeUserRoleSource(source)
	if err := r.assertUserExists(ctx, userID); err != nil {
		return err
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	now := time.Now()
	var existing userRoleModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := r.db.WithContext(ctx).Create(&userRoleModel{
			BaseModel: database.BaseModel{CreatedAt: now, UpdatedAt: now},
			UserID:    userID,
			RoleID:    roleID,
			Source:    string(source),
		}).Error; err != nil {
			return err
		}
		r.invalidateUserGrantCache(ctx, userID)
		return nil
	}
	if err != nil {
		return err
	}
	nextSource := domain.PreserveUserRoleSource(domain.UserRoleSource(existing.Source), source)
	if err := r.db.WithContext(ctx).Model(&userRoleModel{}).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Updates(map[string]any{
			"source":     string(nextSource),
			"updated_at": now,
		}).Error; err != nil {
		return err
	}
	r.invalidateUserGrantCache(ctx, userID)
	return nil
}

func (r *AccessControlRepository) UnbindUserRole(ctx context.Context, userID, roleID string) error {
	userID = strings.TrimSpace(userID)
	roleID = strings.TrimSpace(roleID)
	if userID == "" || roleID == "" {
		return fmt.Errorf("userID and roleID are required")
	}
	res := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Delete(&userRoleModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		r.invalidateUserGrantCache(ctx, userID)
	}
	return nil
}

func (r *AccessControlRepository) BindRolePermission(ctx context.Context, roleID, permissionID string) error {
	roleID = strings.TrimSpace(roleID)
	permissionID = strings.TrimSpace(permissionID)
	if roleID == "" || permissionID == "" {
		return fmt.Errorf("roleID and permissionID are required")
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	if err := r.assertPermissionExists(ctx, permissionID); err != nil {
		return err
	}
	return r.upsertAssociation(ctx, &rolePermissionModel{
		BaseModel:    database.BaseModel{CreatedAt: time.Now(), UpdatedAt: time.Now()},
		RoleID:       roleID,
		PermissionID: permissionID,
	}, []string{"role_id", "permission_id"})
}

func (r *AccessControlRepository) BindRoleDataScope(ctx context.Context, roleID, dataScopeID string) error {
	roleID = strings.TrimSpace(roleID)
	dataScopeID = strings.TrimSpace(dataScopeID)
	if roleID == "" || dataScopeID == "" {
		return fmt.Errorf("roleID and dataScopeID are required")
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	if err := r.assertDataScopeExists(ctx, dataScopeID); err != nil {
		return err
	}
	return r.upsertAssociation(ctx, &roleDataScopeModel{
		BaseModel:   database.BaseModel{CreatedAt: time.Now(), UpdatedAt: time.Now()},
		RoleID:      roleID,
		DataScopeID: dataScopeID,
	}, []string{"role_id", "data_scope_id"})
}

func (r *AccessControlRepository) BindRoleAIToolPermission(ctx context.Context, roleID, toolPermissionID string) error {
	roleID = strings.TrimSpace(roleID)
	toolPermissionID = strings.TrimSpace(toolPermissionID)
	if roleID == "" || toolPermissionID == "" {
		return fmt.Errorf("roleID and toolPermissionID are required")
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	if err := r.assertAIToolPermissionExists(ctx, toolPermissionID); err != nil {
		return err
	}
	return r.upsertAssociation(ctx, &roleAIToolPermissionModel{
		BaseModel:        database.BaseModel{CreatedAt: time.Now(), UpdatedAt: time.Now()},
		RoleID:           roleID,
		ToolPermissionID: toolPermissionID,
	}, []string{"role_id", "tool_permission_id"})
}

func (r *AccessControlRepository) ReplaceUserManualRoles(ctx context.Context, userID string, roleIDs []string) error {
	userID = strings.TrimSpace(userID)
	roleIDs = cleanIDs(roleIDs)
	if userID == "" {
		return fmt.Errorf("userID is required")
	}
	if err := r.assertUserExists(ctx, userID); err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		if err := r.assertRoleExists(ctx, roleID); err != nil {
			return err
		}
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND source = ?", userID, string(domain.UserRoleSourceManual)).
			Delete(&userRoleModel{}).Error; err != nil {
			return err
		}
		var preserved []userRoleModel
		if err := tx.Where("user_id = ? AND source <> ?", userID, string(domain.UserRoleSourceManual)).
			Find(&preserved).Error; err != nil {
			return err
		}
		preservedRoleIDs := make(map[string]struct{}, len(preserved))
		for _, binding := range preserved {
			preservedRoleIDs[binding.RoleID] = struct{}{}
		}
		now := time.Now()
		for _, roleID := range roleIDs {
			if _, ok := preservedRoleIDs[roleID]; ok {
				continue
			}
			if err := tx.Create(&userRoleModel{
				BaseModel: database.BaseModel{CreatedAt: now, UpdatedAt: now},
				UserID:    userID,
				RoleID:    roleID,
				Source:    string(domain.UserRoleSourceManual),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		r.invalidateUserGrantCache(ctx, userID)
	}
	return err
}

func (r *AccessControlRepository) ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error {
	roleID = strings.TrimSpace(roleID)
	permissionIDs = cleanIDs(permissionIDs)
	if roleID == "" {
		return fmt.Errorf("roleID is required")
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	for _, permissionID := range permissionIDs {
		if err := r.assertPermissionExists(ctx, permissionID); err != nil {
			return err
		}
	}
	err := r.replaceRoleAssociations(ctx, roleID, func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&rolePermissionModel{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, permissionID := range permissionIDs {
			if err := tx.Create(&rolePermissionModel{
				BaseModel:    database.BaseModel{CreatedAt: now, UpdatedAt: now},
				RoleID:       roleID,
				PermissionID: permissionID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func (r *AccessControlRepository) ReplaceRoleDataScopes(ctx context.Context, roleID string, dataScopeIDs []string) error {
	roleID = strings.TrimSpace(roleID)
	dataScopeIDs = cleanIDs(dataScopeIDs)
	if roleID == "" {
		return fmt.Errorf("roleID is required")
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	for _, dataScopeID := range dataScopeIDs {
		if err := r.assertDataScopeExists(ctx, dataScopeID); err != nil {
			return err
		}
	}
	return r.replaceRoleAssociations(ctx, roleID, func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&roleDataScopeModel{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, dataScopeID := range dataScopeIDs {
			if err := tx.Create(&roleDataScopeModel{
				BaseModel:   database.BaseModel{CreatedAt: now, UpdatedAt: now},
				RoleID:      roleID,
				DataScopeID: dataScopeID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AccessControlRepository) ReplaceRoleAIToolPermissions(ctx context.Context, roleID string, toolPermissionIDs []string) error {
	roleID = strings.TrimSpace(roleID)
	toolPermissionIDs = cleanIDs(toolPermissionIDs)
	if roleID == "" {
		return fmt.Errorf("roleID is required")
	}
	if err := r.assertRoleExists(ctx, roleID); err != nil {
		return err
	}
	for _, toolPermissionID := range toolPermissionIDs {
		if err := r.assertAIToolPermissionExists(ctx, toolPermissionID); err != nil {
			return err
		}
	}
	return r.replaceRoleAssociations(ctx, roleID, func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&roleAIToolPermissionModel{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for _, toolPermissionID := range toolPermissionIDs {
			if err := tx.Create(&roleAIToolPermissionModel{
				BaseModel:        database.BaseModel{CreatedAt: now, UpdatedAt: now},
				RoleID:           roleID,
				ToolPermissionID: toolPermissionID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AccessControlRepository) replaceRoleAssociations(ctx context.Context, roleID string, fn func(*gorm.DB) error) error {
	err := r.db.WithContext(ctx).Transaction(fn)
	if err == nil {
		r.invalidateRoleGrantCache(ctx, roleID)
	}
	return err
}

func (r *AccessControlRepository) upsertAssociation(ctx context.Context, row any, conflictColumns []string) error {
	cols := make([]clause.Column, 0, len(conflictColumns))
	for _, name := range conflictColumns {
		cols = append(cols, clause.Column{Name: name})
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   cols,
			DoUpdates: clause.AssignmentColumns([]string{"updated_at"}),
		}).
		Create(row).Error
}

func cleanIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (r *AccessControlRepository) assertUserExists(ctx context.Context, userID string) error {
	ok, err := r.existsByBusinessID(ctx, &userModel{}, "user_id", userID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: user %q", domain.ErrReferenceNotFound, userID)
	}
	return nil
}

func (r *AccessControlRepository) assertRoleExists(ctx context.Context, roleID string) error {
	ok, err := r.existsByBusinessID(ctx, &roleModel{}, "role_id", roleID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: role %q", domain.ErrReferenceNotFound, roleID)
	}
	return nil
}

func (r *AccessControlRepository) assertPermissionExists(ctx context.Context, permissionID string) error {
	ok, err := r.existsByBusinessID(ctx, &permissionModel{}, "permission_id", permissionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: permission %q", domain.ErrReferenceNotFound, permissionID)
	}
	return nil
}

func (r *AccessControlRepository) assertDataScopeExists(ctx context.Context, dataScopeID string) error {
	ok, err := r.existsByBusinessID(ctx, &dataScopeModel{}, "data_scope_id", dataScopeID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: data scope %q", domain.ErrReferenceNotFound, dataScopeID)
	}
	return nil
}

func (r *AccessControlRepository) assertAIToolPermissionExists(ctx context.Context, toolPermissionID string) error {
	ok, err := r.existsByBusinessID(ctx, &aiToolPermissionModel{}, "tool_permission_id", toolPermissionID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: ai tool permission %q", domain.ErrReferenceNotFound, toolPermissionID)
	}
	return nil
}

func (r *AccessControlRepository) existsByBusinessID(ctx context.Context, model any, column, value string) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("access control repository is not configured")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(model).Where(column+" = ?", value).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AccessControlRepository) HasUserRole(ctx context.Context, userID, roleID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	roleID = strings.TrimSpace(roleID)
	if userID == "" || roleID == "" {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&userRoleModel{}).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AccessControlRepository) ListRolePermissions(ctx context.Context, roleID string) ([]domain.Permission, error) {
	var rows []permissionModel
	if err := r.db.WithContext(ctx).
		Table("iam_permission").
		Select("iam_permission.*").
		Joins("JOIN iam_role_permission ON iam_role_permission.permission_id = iam_permission.permission_id").
		Where("iam_role_permission.role_id = ?", strings.TrimSpace(roleID)).
		Order("iam_permission.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapPermissions(rows), nil
}

func (r *AccessControlRepository) ListDataScopes(ctx context.Context, filter domain.DataScopeFilter) ([]domain.DataScope, error) {
	var rows []dataScopeModel
	q := r.db.WithContext(ctx).Model(&dataScopeModel{})
	if filter.ScopeType != nil {
		q = q.Where("scope_type = ?", string(*filter.ScopeType))
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return mapDataScopes(rows), nil
}

func (r *AccessControlRepository) FindDataScopeByCode(ctx context.Context, code string) (*domain.DataScope, error) {
	return r.findDataScope(ctx, "code = ?", code)
}

func (r *AccessControlRepository) FindDataScopeByID(ctx context.Context, id string) (*domain.DataScope, error) {
	return r.findDataScope(ctx, "data_scope_id = ?", id)
}

func (r *AccessControlRepository) findDataScope(ctx context.Context, cond string, arg any) (*domain.DataScope, error) {
	var row dataScopeModel
	if err := r.db.WithContext(ctx).First(&row, cond, arg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainDataScope(&row), nil
}

func (r *AccessControlRepository) ListRoleDataScopes(ctx context.Context, roleID string) ([]domain.DataScope, error) {
	var rows []dataScopeModel
	if err := r.db.WithContext(ctx).
		Table("iam_data_scope").
		Select("iam_data_scope.*").
		Joins("JOIN iam_role_data_scope ON iam_role_data_scope.data_scope_id = iam_data_scope.data_scope_id").
		Where("iam_role_data_scope.role_id = ?", strings.TrimSpace(roleID)).
		Order("iam_data_scope.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapDataScopes(rows), nil
}

func (r *AccessControlRepository) ListAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) ([]domain.AIToolPermission, error) {
	var rows []aiToolPermissionModel
	q := r.db.WithContext(ctx).Model(&aiToolPermissionModel{})
	if filter.PermissionMode != nil {
		q = q.Where("permission_mode = ?", string(*filter.PermissionMode))
	}
	if err := q.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return mapAIToolPermissions(rows), nil
}

func (r *AccessControlRepository) CountAIToolPermissions(ctx context.Context, filter domain.AIToolPermissionFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&aiToolPermissionModel{})
	if filter.PermissionMode != nil {
		q = q.Where("permission_mode = ?", string(*filter.PermissionMode))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *AccessControlRepository) FindAIToolPermissionByCode(ctx context.Context, code string) (*domain.AIToolPermission, error) {
	return r.findAIToolPermission(ctx, "tool_code = ?", code)
}

func (r *AccessControlRepository) FindAIToolPermissionByID(ctx context.Context, id string) (*domain.AIToolPermission, error) {
	return r.findAIToolPermission(ctx, "tool_permission_id = ?", id)
}

func (r *AccessControlRepository) findAIToolPermission(ctx context.Context, cond string, arg any) (*domain.AIToolPermission, error) {
	var row aiToolPermissionModel
	if err := r.db.WithContext(ctx).First(&row, cond, arg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainAIToolPermission(&row), nil
}

func (r *AccessControlRepository) ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]domain.AIToolPermission, error) {
	var rows []aiToolPermissionModel
	if err := r.db.WithContext(ctx).
		Table("iam_ai_tool_permission").
		Select("iam_ai_tool_permission.*").
		Joins("JOIN iam_role_ai_tool_permission ON iam_role_ai_tool_permission.tool_permission_id = iam_ai_tool_permission.tool_permission_id").
		Where("iam_role_ai_tool_permission.role_id = ?", strings.TrimSpace(roleID)).
		Order("iam_ai_tool_permission.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapAIToolPermissions(rows), nil
}

func (r *AccessControlRepository) LoadUserGrantContext(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return &domain.UserGrantContext{}, nil
	}
	if r.grantCache != nil {
		if cached, ok, err := r.grantCache.get(ctx, userID); err != nil {
			logger.From(ctx).Warn("user grant cache get failed, falling back to database",
				zap.String("user_id", userID),
				zap.Error(err),
			)
		} else if ok {
			return cached, nil
		}
	}
	loader := r.loadUserGrantContextFromDB
	if r.loadUserGrantContext != nil {
		loader = r.loadUserGrantContext
	}
	grant, err := loader(ctx, userID)
	if err != nil {
		return nil, err
	}
	if r.grantCache != nil {
		if err := r.grantCache.set(ctx, userID, grant); err != nil {
			logger.From(ctx).Warn("user grant cache set failed",
				zap.String("user_id", userID),
				zap.Error(err),
			)
		}
	}
	return grant, nil
}

func (r *AccessControlRepository) loadUserGrantContextFromDB(ctx context.Context, userID string) (*domain.UserGrantContext, error) {
	roles, err := r.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := &domain.UserGrantContext{Roles: roles}
	if len(roles) == 0 {
		return out, nil
	}
	roleIDs := make([]string, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.ID)
	}
	perms, err := r.listPermissionsForRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	scopes, err := r.listDataScopesForRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	tools, err := r.listAIToolPermissionsForRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	out.Permissions = perms
	out.DataScopes = scopes
	out.AIToolPermissions = tools
	return out, nil
}

func (r *AccessControlRepository) invalidateUserGrantCache(ctx context.Context, userID string) {
	if r == nil || r.grantCache == nil {
		return
	}
	if err := r.grantCache.delete(ctx, userID); err != nil {
		logger.From(ctx).Warn("user grant cache invalidate failed",
			zap.String("user_id", userID),
			zap.Error(err),
		)
	}
}

func (r *AccessControlRepository) invalidateRoleGrantCache(ctx context.Context, roleID string) {
	if r == nil || r.grantCache == nil {
		return
	}
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return
	}
	var rows []userRoleModel
	if err := r.db.WithContext(ctx).
		Where("role_id = ?", roleID).
		Find(&rows).Error; err != nil {
		logger.From(ctx).Warn("role grant cache users load failed",
			zap.String("role_id", roleID),
			zap.Error(err),
		)
		return
	}
	for _, row := range rows {
		r.invalidateUserGrantCache(ctx, row.UserID)
	}
}

func (r *AccessControlRepository) listPermissionsForRoleIDs(ctx context.Context, roleIDs []string) ([]domain.Permission, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var rows []permissionModel
	if err := r.db.WithContext(ctx).
		Table("iam_permission").
		Select("DISTINCT iam_permission.*").
		Joins("JOIN iam_role_permission ON iam_role_permission.permission_id = iam_permission.permission_id").
		Where("iam_role_permission.role_id IN ?", roleIDs).
		Order("iam_permission.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapPermissions(rows), nil
}

func (r *AccessControlRepository) listDataScopesForRoleIDs(ctx context.Context, roleIDs []string) ([]domain.DataScope, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var rows []dataScopeModel
	if err := r.db.WithContext(ctx).
		Table("iam_data_scope").
		Select("DISTINCT iam_data_scope.*").
		Joins("JOIN iam_role_data_scope ON iam_role_data_scope.data_scope_id = iam_data_scope.data_scope_id").
		Where("iam_role_data_scope.role_id IN ?", roleIDs).
		Order("iam_data_scope.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapDataScopes(rows), nil
}

func (r *AccessControlRepository) listAIToolPermissionsForRoleIDs(ctx context.Context, roleIDs []string) ([]domain.AIToolPermission, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var rows []aiToolPermissionModel
	if err := r.db.WithContext(ctx).
		Table("iam_ai_tool_permission").
		Select("DISTINCT iam_ai_tool_permission.*").
		Joins("JOIN iam_role_ai_tool_permission ON iam_role_ai_tool_permission.tool_permission_id = iam_ai_tool_permission.tool_permission_id").
		Where("iam_role_ai_tool_permission.role_id IN ?", roleIDs).
		Order("iam_ai_tool_permission.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return mapAIToolPermissions(rows), nil
}

func mapRoles(rows []roleModel) []domain.Role {
	out := make([]domain.Role, 0, len(rows))
	for i := range rows {
		out = append(out, *toDomainRole(&rows[i]))
	}
	return out
}
func mapPermissions(rows []permissionModel) []domain.Permission {
	out := make([]domain.Permission, 0, len(rows))
	for i := range rows {
		out = append(out, *toDomainPermission(&rows[i]))
	}
	return out
}
func mapDataScopes(rows []dataScopeModel) []domain.DataScope {
	out := make([]domain.DataScope, 0, len(rows))
	for i := range rows {
		out = append(out, *toDomainDataScope(&rows[i]))
	}
	return out
}
func mapAIToolPermissions(rows []aiToolPermissionModel) []domain.AIToolPermission {
	out := make([]domain.AIToolPermission, 0, len(rows))
	for i := range rows {
		out = append(out, *toDomainAIToolPermission(&rows[i]))
	}
	return out
}

func toDomainUserRole(m *userRoleModel) domain.UserRole {
	if m == nil {
		return domain.UserRole{}
	}
	return domain.UserRole{
		UserID:    m.UserID,
		RoleID:    m.RoleID,
		Source:    domain.UserRoleSource(m.Source),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func toDomainRole(m *roleModel) *domain.Role {
	if m == nil {
		return nil
	}
	return &domain.Role{ID: m.RoleID, Code: m.Code, Name: m.Name, Description: m.Description, Status: domain.RoleStatus(m.Status), IsSystem: m.IsSystem, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func toDomainPermission(m *permissionModel) *domain.Permission {
	if m == nil {
		return nil
	}
	return &domain.Permission{ID: m.PermissionID, Code: m.Code, Name: m.Name, Resource: m.Resource, Action: m.Action, Description: m.Description, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func toDomainDataScope(m *dataScopeModel) *domain.DataScope {
	if m == nil {
		return nil
	}
	cfg := map[string]any{}
	_ = json.Unmarshal(m.ScopeConfig, &cfg)
	return &domain.DataScope{ID: m.DataScopeID, Code: m.Code, Name: m.Name, ScopeType: domain.DataScopeType(m.ScopeType), ScopeConfig: cfg, Description: m.Description, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func toDomainAIToolPermission(m *aiToolPermissionModel) *domain.AIToolPermission {
	if m == nil {
		return nil
	}
	return &domain.AIToolPermission{ID: m.ToolPermissionID, ToolCode: m.ToolCode, ToolName: m.ToolName, PermissionMode: domain.AIToolPermissionMode(m.PermissionMode), PermitsUnconfirmedInvoke: m.AllowConfirm, Description: m.Description, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
