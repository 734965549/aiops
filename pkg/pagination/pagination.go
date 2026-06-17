// Package pagination 提供平台统一的分页参数与结果结构。
//
// 契约 PageData<T>（ops/alert-contract.md §2、ops/identity-api-contract.md §5.3）
// 在本包实现为 Result[T]：items / total / page / page_size。
//
// 约定：page 从 1 开始；默认 page_size=20；最大 page_size=100（Normalize 自动修正）。
package pagination

const (
	defaultPage     = 1   // §2：页码从 1 开始
	defaultPageSize = 20  // §8.1 告警列表默认每页条数
	maxPageSize     = 100 // §2：page_size 上限
)

// Query 描述分页查询参数（前端 query/form 传入，对应 §2 PageData 的 page / page_size）。
type Query struct {
	Page     int    `form:"page" json:"page"`
	PageSize int    `form:"page_size" json:"page_size"`
	Keyword  string `form:"keyword" json:"keyword"`
	OrderBy  string `form:"order_by" json:"order_by"`
}

// Normalize 修正页码/页大小到合理范围（§2：page≥1，默认 20，最大 100）。
func (q *Query) Normalize() {
	if q.Page <= 0 {
		q.Page = defaultPage
	}
	if q.PageSize <= 0 {
		q.PageSize = defaultPageSize
	}
	if q.PageSize > maxPageSize {
		q.PageSize = maxPageSize
	}
}

// Offset 计算 SQL OFFSET。
func (q Query) Offset() int { return (q.Page - 1) * q.PageSize }

// Limit 返回 SQL LIMIT。
func (q Query) Limit() int { return q.PageSize }

// Result 是分页响应载荷（契约 PageData<T>，见 ops/alert-contract.md §2、ops/identity-api-contract.md §5.3）。
type Result[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// NewResult 组装 §2 PageData 载荷，供 httpx.OK(c, ...) 写入 data 字段。
func NewResult[T any](items []T, total int64, q Query) Result[T] {
	return Result[T]{
		Items:    items,
		Total:    total,
		Page:     q.Page,
		PageSize: q.PageSize,
	}
}
