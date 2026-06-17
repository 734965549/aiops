package domain

import "context"

// TemplateFilter 模板列表筛选。
type TemplateFilter struct {
	Enabled *bool
	Keyword string
	Limit   int
	Offset  int
}

// TemplateRepository 模板仓储。
type TemplateRepository interface {
	Create(ctx context.Context, tpl *Template) error
	CreateWithSteps(ctx context.Context, tpl *Template, steps []Step) error
	Update(ctx context.Context, tpl *Template) error
	ReplaceWithSteps(ctx context.Context, tpl *Template, steps []Step) error
	Delete(ctx context.Context, templateID string) error
	GetByID(ctx context.Context, templateID string) (*Template, error)
	List(ctx context.Context, filter TemplateFilter) ([]Template, error)
	Count(ctx context.Context, filter TemplateFilter) (int64, error)
	ListEnabled(ctx context.Context) ([]Template, error)
}

// StepRepository 步骤仓储。
type StepRepository interface {
	Create(ctx context.Context, step *Step) error
	Update(ctx context.Context, step *Step) error
	DeleteByTemplateID(ctx context.Context, templateID string) error
	ListByTemplateID(ctx context.Context, templateID string) ([]Step, error)
}
