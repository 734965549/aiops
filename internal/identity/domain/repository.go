package domain

import "context"

// UserRepository 定义用户聚合的持久化能力，由 infrastructure 实现。
//
// 仓储约定：
//   - 查询找不到记录时返回 (nil, nil)，由上层决定是 NOT_FOUND 还是其它分支；
//   - Create 在用户名 / 业务 ID 冲突时返回 ErrAlreadyExists；
//   - Update 不会重置时间戳，依赖 GORM Hook 由 BaseModel 维护 updated_at。
type UserRepository interface {
	FindByID(ctx context.Context, id string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	DeleteByID(ctx context.Context, id string) error
}

// ExternalIdentityRepository 管理外部身份绑定。
type ExternalIdentityRepository interface {
	FindByProviderSubject(ctx context.Context, providerID, externalSubject string) (*ExternalIdentity, error)
	FindByUserAndProvider(ctx context.Context, userID, providerID string) (*ExternalIdentity, error)
	Create(ctx context.Context, ext *ExternalIdentity) error
	Update(ctx context.Context, ext *ExternalIdentity) error
	DeleteByProviderSubject(ctx context.Context, providerID, externalSubject string) error
}

// AuthAuditRepository 记录认证审计事件，俾登录链路同管理员查询共用。
type AuthAuditRepository interface {
	Create(ctx context.Context, audit *AuthAudit) error
	List(ctx context.Context, filter AuthAuditFilter) ([]AuthAudit, error)
	Count(ctx context.Context, filter AuthAuditFilter) (int64, error)
}

// AccessControlRepository 提供 Identity 权限域的查询能力。
//
// 关联表写入（Bind*）在 INSERT 前校验主记录存在，替代数据库外键约束。
type AccessControlRepository interface {
	ListRoles(ctx context.Context, filter RoleFilter) ([]Role, error)
	CountRoles(ctx context.Context, filter RoleFilter) (int64, error)
	FindRoleByID(ctx context.Context, id string) (*Role, error)
	FindRoleByCode(ctx context.Context, code string) (*Role, error)
	ListPermissions(ctx context.Context, filter PermissionFilter) ([]Permission, error)
	CountPermissions(ctx context.Context, filter PermissionFilter) (int64, error)
	FindPermissionByID(ctx context.Context, id string) (*Permission, error)
	FindPermissionByCode(ctx context.Context, code string) (*Permission, error)
	ListUserRoles(ctx context.Context, userID string) ([]Role, error)
	ListUserRoleBindings(ctx context.Context, userID string) ([]UserRole, error)
	BindUserRole(ctx context.Context, userID, roleID string, source UserRoleSource) error
	UnbindUserRole(ctx context.Context, userID, roleID string) error
	BindRolePermission(ctx context.Context, roleID, permissionID string) error
	BindRoleDataScope(ctx context.Context, roleID, dataScopeID string) error
	BindRoleAIToolPermission(ctx context.Context, roleID, toolPermissionID string) error
	HasUserRole(ctx context.Context, userID, roleID string) (bool, error)
	ListRolePermissions(ctx context.Context, roleID string) ([]Permission, error)
	ListDataScopes(ctx context.Context, filter DataScopeFilter) ([]DataScope, error)
	FindDataScopeByCode(ctx context.Context, code string) (*DataScope, error)
	FindDataScopeByID(ctx context.Context, id string) (*DataScope, error)
	ListRoleDataScopes(ctx context.Context, roleID string) ([]DataScope, error)
	ListAIToolPermissions(ctx context.Context, filter AIToolPermissionFilter) ([]AIToolPermission, error)
	CountAIToolPermissions(ctx context.Context, filter AIToolPermissionFilter) (int64, error)
	FindAIToolPermissionByCode(ctx context.Context, code string) (*AIToolPermission, error)
	FindAIToolPermissionByID(ctx context.Context, id string) (*AIToolPermission, error)
	ListRoleAIToolPermissions(ctx context.Context, roleID string) ([]AIToolPermission, error)
	ReplaceUserManualRoles(ctx context.Context, userID string, roleIDs []string) error
	ReplaceRolePermissions(ctx context.Context, roleID string, permissionIDs []string) error
	ReplaceRoleDataScopes(ctx context.Context, roleID string, dataScopeIDs []string) error
	ReplaceRoleAIToolPermissions(ctx context.Context, roleID string, toolPermissionIDs []string) error
	// LoadUserGrantContext 按 userID 批量加载授权所需的全部 RBAC 数据，避免按角色 N+1 查询。
	LoadUserGrantContext(ctx context.Context, userID string) (*UserGrantContext, error)
}

// UserGrantContext 聚合用户角色及其关联的权限、数据范围与 AI 工具权限。
type UserGrantContext struct {
	Roles             []Role
	Permissions       []Permission
	DataScopes        []DataScope
	AIToolPermissions []AIToolPermission
}

// RoleFilter 角色查询过滤器。
type RoleFilter struct {
	Status   *RoleStatus
	IsSystem *bool
	Limit    int
	Offset   int
}

// PermissionFilter 权限查询过滤器。
type PermissionFilter struct {
	Resource string
	Action   string
	Limit    int
	Offset   int
}

// DataScopeFilter 数据范围查询过滤器。
type DataScopeFilter struct {
	ScopeType *DataScopeType
}

// AIToolPermissionFilter AI 工具权限查询过滤器。
type AIToolPermissionFilter struct {
	PermissionMode *AIToolPermissionMode
}
