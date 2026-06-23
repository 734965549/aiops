package huawei

import (
	"testing"

	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"

	"github.com/734965549/aiops/internal/observability/domain"
)

func TestMapCloudResourceToAssetFields(t *testing.T) {
	resourceType, instance := MapCloudResourceToAssetFields(domain.CloudResource{
		Type:        "ecs",
		ProviderRef: "ecs-123",
		Labels:      map[string]string{"instance_id": "ecs-123"},
	})
	if resourceType != "host" || instance != "ecs-123" {
		t.Fatalf("ecs mapping: type=%s instance=%s", resourceType, instance)
	}

	resourceType, instance = MapCloudResourceToAssetFields(domain.CloudResource{
		Type:        "rds",
		ProviderRef: "rds-abc",
	})
	if resourceType != "service" || instance != "rds-abc" {
		t.Fatalf("rds mapping: type=%s instance=%s", resourceType, instance)
	}
}

func TestMapECSServer(t *testing.T) {
	res := mapECSServer("cn-north-4", ecsmodel.ServerDetail{
		Id: "srv-1", Name: "web-01", Status: "ACTIVE",
	})
	if res.Type != "ecs" || res.Region != "cn-north-4" || res.ProviderRef != "srv-1" {
		t.Fatalf("unexpected ecs mapping: %+v", res)
	}
	if res.Labels["instance_id"] != "srv-1" {
		t.Fatalf("expected instance_id label, got %+v", res.Labels)
	}
}

func TestMapRDSInstance(t *testing.T) {
	res := mapRDSInstance("cn-north-4", rdsmodel.InstanceResponse{
		Id: "rds-1", Name: "db-main", Status: "ACTIVE",
	})
	if res.Type != "rds" || res.ProviderRef != "rds-1" || res.Name != "db-main" {
		t.Fatalf("unexpected rds mapping: %+v", res)
	}
}
