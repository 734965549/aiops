package huawei

import (
	"strings"
	"time"

	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdktime"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

var primaryAlarmDimensions = []string{
	"instance_id", "resource_id", "disk_name", "lbaas_instance_id",
	"bucket_name", "kafka_broker", "publicip_id", "rds_cluster_id", "dcs_instance_id",
}

// mapAlarmHistory 将 CES V2 AlarmHistoryItemV2 映射为平台告警记录（契约 §6.2）。
func mapAlarmHistory(item cesv2model.AlarmHistoryItemV2, scope domain.AlarmPullScope) domain.AlarmRecord {
	rec := domain.AlarmRecord{
		RecordID:    strings.TrimSpace(valueOf(item.RecordId)),
		AlarmID:     strings.TrimSpace(valueOf(item.AlarmId)),
		Name:        strings.TrimSpace(valueOf(item.Name)),
		Status:      statusOf(item.Status),
		Level:       levelString(item.Level),
		FirstSeenAt: firstNonNullTime(item.FirstAlarmTime, item.BeginTime),
		LastAlarmAt: sdkTimePtr(item.LastAlarmTime),
		RecoveredAt: sdkTimePtr(item.AlarmRecoveryTime),
		Dimensions:  map[string]string{},
	}
	if item.Metric != nil {
		rec.Namespace = strings.TrimSpace(valueOf(item.Metric.Namespace))
		rec.MetricName = strings.TrimSpace(valueOf(item.Metric.MetricName))
		rec.ResourceType = resourceTypeFromNamespace(rec.Namespace)
		if item.Metric.Dimensions != nil {
			for _, dim := range *item.Metric.Dimensions {
				name := strings.TrimSpace(valueOf(dim.Name))
				value := strings.TrimSpace(valueOf(dim.Value))
				if name == "" || value == "" {
					continue
				}
				rec.Dimensions[name] = value
			}
		}
	}
	if item.Condition != nil {
		rec.Condition = mapAlarmCondition(item.Condition)
	}
	if item.AdditionalInfo != nil {
		rec.ResourceID = strings.TrimSpace(valueOf(item.AdditionalInfo.ResourceId))
		rec.ResourceName = strings.TrimSpace(valueOf(item.AdditionalInfo.ResourceName))
	}
	if rec.ResourceID == "" {
		rec.ResourceID = primaryDimensionValue(rec.Dimensions)
	}
	if rec.ResourceName == "" {
		rec.ResourceName = rec.ResourceID
	}
	_ = scope
	return rec
}

// mapAlarmCondition 映射触发条件；字段缺失时返回 nil，不构造空对象。
func mapAlarmCondition(in *cesv2model.AlarmHistoryItemV2Condition) *domain.AlarmCondition {
	if in == nil {
		return nil
	}
	out := &domain.AlarmCondition{
		Filter:             strings.TrimSpace(valueOf(in.Filter)),
		ComparisonOperator: strings.TrimSpace(valueOf(in.ComparisonOperator)),
		Unit:               strings.TrimSpace(valueOf(in.Unit)),
	}
	if in.Value != nil {
		out.Threshold = *in.Value
	}
	if in.Count != nil {
		out.TriggerCount = int(*in.Count)
	}
	if in.Period != nil {
		out.PeriodSeconds = int64(in.Period.Value())
	}
	if in.SuppressDuration != nil {
		out.SuppressDurationS = int64(in.SuppressDuration.Value())
	}
	if out.ComparisonOperator == "" && out.Threshold == 0 {
		return nil
	}
	return out
}

func statusOf(status *cesv2model.AlarmHistoryItemV2Status) string {
	if status == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(status.Value()))
}

func sdkTimePtr(t *sdktime.SdkTime) *time.Time {
	if t == nil {
		return nil
	}
	v := time.Time(*t).UTC()
	if v.IsZero() {
		return nil
	}
	return &v
}

func firstNonNullTime(values ...*sdktime.SdkTime) *time.Time {
	for _, v := range values {
		if t := sdkTimePtr(v); t != nil {
			return t
		}
	}
	return nil
}

func primaryDimensionValue(dims map[string]string) string {
	for _, key := range primaryAlarmDimensions {
		if v := strings.TrimSpace(dims[key]); v != "" {
			return v
		}
	}
	for _, v := range dims {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func resourceTypeFromNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return ""
	}
	parts := strings.Split(namespace, ".")
	res := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	switch res {
	case "elb":
		return "elb"
	case "evs":
		return "evs"
	case "obs":
		return "obs"
	case "vpc":
		return "vpc"
	case "rds":
		return "rds"
	case "dcs":
		return "dcs"
	case "dms":
		return "dms"
	default:
		return res
	}
}
