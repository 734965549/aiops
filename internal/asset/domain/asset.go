// Package domain 定义 Asset 资产管理领域模型与仓储接口（Alert §9.1 标签匹配）。
package domain

import "time"

// Application 平台注册嘅应用/服务，供 Alert 接入时匹配 application_id。
type Application struct {
	ID          string // 业务 ID（UUID）
	Name        string // 应用名，如 payment-service
	Environment string // 环境 prod/staging/dev
	Namespace   string // 默认 K8s namespace（可选）
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Resource 平台注册嘅资源实例，供 Alert 接入时匹配 resource_id。
type Resource struct {
	ID            string // 业务 ID（UUID）
	ApplicationID string // 所属应用 ID
	Name          string // 资源显示名
	ResourceType  string // host/pod/service 等
	Namespace     string // K8s namespace
	Pod           string // pod 名
	Node          string // node 名
	Instance      string // Prometheus instance 等
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
