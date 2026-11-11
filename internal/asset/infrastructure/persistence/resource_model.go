package persistence

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/asset/domain"
	"github.com/734965549/aiops/pkg/database"
)

type resourceModel struct {
	database.BaseModel
	ResourceID           string     `gorm:"column:resource_id;type:varchar(36);uniqueIndex;not null"`
	ApplicationID        string     `gorm:"column:application_id;type:varchar(36);not null;index"`
	Name                 string     `gorm:"column:name;type:varchar(255);not null;default:''"`
	ResourceType         string     `gorm:"column:resource_type;type:varchar(64);not null;default:''"`
	Namespace            string     `gorm:"column:namespace;type:varchar(128);not null;default:'';index:idx_asset_resource_match,priority:2"`
	Pod                  string     `gorm:"column:pod;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:3"`
	Node                 string     `gorm:"column:node;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:4"`
	Instance             string     `gorm:"column:instance;type:varchar(255);not null;default:'';index:idx_asset_resource_match,priority:5"`
	Source               string     `gorm:"column:source;type:varchar(32);not null;default:manual"`
	IntegrationAccountID string     `gorm:"column:integration_account_id;type:varchar(64);not null;default:'';index:idx_asset_resource_cloud_key,priority:1"`
	CloudResourceID      string     `gorm:"column:cloud_resource_id;type:varchar(255);not null;default:'';index:idx_asset_resource_cloud_key,priority:3"`
	CloudResourceType    string     `gorm:"column:cloud_resource_type;type:varchar(64);not null;default:'';index:idx_asset_resource_cloud_key,priority:2"`
	Region               string     `gorm:"column:region;type:varchar(64);not null;default:'';index:idx_asset_resource_cloud_key,priority:4"`
	SyncStatus           string     `gorm:"column:sync_status;type:varchar(32);not null;default:''"`
	LastSyncedAt         *time.Time `gorm:"column:last_synced_at"`
	SyncBatchID          string     `gorm:"column:sync_batch_id;type:varchar(64);not null;default:''"`
	Labels               []byte     `gorm:"column:labels;type:jsonb;not null;default:'{}'::jsonb"`
}

func (resourceModel) TableName() string { return "asset_resource" }

func defaultSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return domain.ResourceSourceManual
	}
	return source
}

func toResourceModel(res *domain.Resource) resourceModel {
	if res == nil {
		return resourceModel{}
	}
	labels, _ := marshalResourceLabels(res.Labels)
	return resourceModel{
		ResourceID:           res.ID,
		ApplicationID:        res.ApplicationID,
		Name:                 res.Name,
		ResourceType:         res.ResourceType,
		Namespace:            res.Namespace,
		Pod:                  res.Pod,
		Node:                 res.Node,
		Instance:             res.Instance,
		Source:               defaultSource(res.Source),
		IntegrationAccountID: res.IntegrationAccountID,
		CloudResourceID:      res.CloudResourceID,
		CloudResourceType:    res.CloudResourceType,
		Region:               res.Region,
		SyncStatus:           res.SyncStatus,
		LastSyncedAt:         res.LastSyncedAt,
		SyncBatchID:          res.SyncBatchID,
		Labels:               labels,
	}
}

func toResourceDomain(m *resourceModel) domain.Resource {
	if m == nil {
		return domain.Resource{}
	}
	return domain.Resource{
		ID:                   m.ResourceID,
		ApplicationID:        m.ApplicationID,
		Name:                 m.Name,
		ResourceType:         m.ResourceType,
		Namespace:            m.Namespace,
		Pod:                  m.Pod,
		Node:                 m.Node,
		Instance:             m.Instance,
		Source:               defaultSource(m.Source),
		IntegrationAccountID: m.IntegrationAccountID,
		CloudResourceID:      m.CloudResourceID,
		CloudResourceType:    m.CloudResourceType,
		Region:               m.Region,
		SyncStatus:           m.SyncStatus,
		LastSyncedAt:         m.LastSyncedAt,
		SyncBatchID:          m.SyncBatchID,
		Labels:               unmarshalResourceLabels(m.Labels),
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
}

// marshalResourceLabels 将 labels map 序列化为 JSONB 字节，空值返回 "{}"。
func marshalResourceLabels(m map[string]string) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// unmarshalResourceLabels 将 JSONB 字节反序列化为 labels map，异常时返回空 map。
func unmarshalResourceLabels(data []byte) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]string{}
	}
	return out
}
