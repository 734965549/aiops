package domain

import "context"

// ResourceMatchQuery 资源匹配查询条件（§9.1：namespace/pod/node/instance/resource_name）。
type ResourceMatchQuery struct {
	ApplicationID string
	Name          string
	ResourceType  string
	Namespace     string
	Pod           string
	Node          string
	Instance      string
}

// ApplicationRepository 应用注册表持久化接口。
type ApplicationRepository interface {
	Create(ctx context.Context, app *Application) error
	List(ctx context.Context) ([]Application, error)
	GetByID(ctx context.Context, id string) (*Application, error)
	Update(ctx context.Context, app *Application) error
	Delete(ctx context.Context, id string) error
	FindByNameEnv(ctx context.Context, name, environment string) (*Application, error)
	ExistsByID(ctx context.Context, id string) (bool, error)
	Count(ctx context.Context) (int64, error)
}

// ResourceRepository 资源注册表持久化接口。
type ResourceRepository interface {
	Create(ctx context.Context, res *Resource) error
	ListByApplicationID(ctx context.Context, applicationID string) ([]Resource, error)
	GetByID(ctx context.Context, id string) (*Resource, error)
	Update(ctx context.Context, res *Resource) error
	Delete(ctx context.Context, id string) error
	CountByApplicationID(ctx context.Context, applicationID string) (int64, error)
	FindBestMatch(ctx context.Context, q ResourceMatchQuery) (*Resource, error)
	Count(ctx context.Context) (int64, error)
}

// MatchRuleRepository 匹配规则持久化接口。
type MatchRuleRepository interface {
	Create(ctx context.Context, rule *MatchRule) error
	List(ctx context.Context) ([]MatchRule, error)
	ListEnabledByPriority(ctx context.Context) ([]MatchRule, error)
	GetByID(ctx context.Context, id string) (*MatchRule, error)
	Update(ctx context.Context, rule *MatchRule) error
	Delete(ctx context.Context, id string) error
	CountByApplicationID(ctx context.Context, applicationID string) (int64, error)
	CountByResourceID(ctx context.Context, resourceID string) (int64, error)
}
