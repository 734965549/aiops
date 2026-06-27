// Package domain 定义 Asset 资产管理领域模型与仓储接口（Alert §9.1 标签匹配）。
package domain

import "time"

// Application 是平台注册的应用/服务，供 Alert 接入时匹配 application_id。
type Application struct {
	ID          string // 业务 ID（UUID）
	Name        string // 应用名，如 payment-service
	Environment string // 环境 prod/staging/dev
	Namespace   string // 默认 K8s namespace（可选）
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const (
	ResourceSourceManual    = "manual"
	ResourceSourceCloudSync = "cloud_sync"

	SyncStatusActive = "active"
	SyncStatusStale  = "stale"
)

// Resource 是平台注册的资源实例，供 Alert 接入时匹配 resource_id。
type Resource struct {
	ID                   string            // 业务 ID（UUID）
	ApplicationID        string            // 所属应用 ID
	Name                 string            // 资源显示名
	ResourceType         string            // host/pod/service 等
	Namespace            string            // K8s namespace
	Pod                  string            // pod 名
	Node                 string            // node 名
	Instance             string            // Prometheus instance 等
	Source               string            // manual / cloud_sync
	IntegrationAccountID string            // 云同步来源账号
	CloudResourceID      string            // 云厂商稳定 ID
	CloudResourceType    string            // ecs/cce/rds/elb
	Region               string            // 区域
	SyncStatus           string            // active/stale（仅 cloud_sync）
	LastSyncedAt         *time.Time        // 最近同步时间
	SyncBatchID          string            // 最近成功批次
	Labels               map[string]string // 云同步标签（CES namespace/dim_name + 原生 API 增强 private_ip/flavor/vpc_id/az），每次同步整体覆盖
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
