package huawei

import (
	"testing"

	"github.com/734965549/aiops/internal/observability/domain"
	cesv2model "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2/model"
)

func TestMapAlarmHistoryWithCondition(t *testing.T) {
	status := cesv2model.GetAlarmHistoryItemV2StatusEnum().ALARM
	level := cesv2model.GetAlarmHistoryItemV2LevelEnum().E_2
	metricName := "disk_usedPercent"
	filter := "average"
	operator := ">"
	value := 80.0
	unit := "%"
	count := int32(3)
	period := cesv2model.GetAlarmHistoryItemV2ConditionPeriodEnum().E_300
	suppress := cesv2model.GetAlarmHistoryItemV2ConditionSuppressDurationEnum().E_600

	item := cesv2model.AlarmHistoryItemV2{
		RecordId: strPtr("ah261111T173939SfELxm8kI"),
		AlarmId:  strPtr("al1754495900067vlRg37lwa"),
		Name:     strPtr("RDS 磁盘使用率告警"),
		Status:   &status,
		Level:    &level,
		Metric: &cesv2model.AlarmHistoryItemV2Metric{
			Namespace:  strPtr("SYS.RDS"),
			MetricName: &metricName,
		},
		Condition: &cesv2model.AlarmHistoryItemV2Condition{
			Filter:             &filter,
			ComparisonOperator: &operator,
			Value:              &value,
			Unit:               &unit,
			Count:              &count,
			Period:             &period,
			SuppressDuration:   &suppress,
		},
		AdditionalInfo: &cesv2model.AdditionalInfo{
			ResourceId:   strPtr("rds-instance-1"),
			ResourceName: strPtr("prod-db"),
		},
	}

	rec := mapAlarmHistory(item, domain.AlarmPullScope{Region: "cn-south-1", ProjectID: "p1"})

	if rec.MetricName != metricName {
		t.Fatalf("metric name = %q, want %q", rec.MetricName, metricName)
	}
	if rec.Namespace != "SYS.RDS" {
		t.Fatalf("namespace = %q", rec.Namespace)
	}
	if rec.Condition == nil {
		t.Fatal("expected condition")
	}
	if got, want := rec.Condition.Describe(), "> 80% (average, 300s x3)"; got != want {
		t.Fatalf("condition describe = %q, want %q", got, want)
	}
	if rec.Condition.SuppressDurationS != 600 {
		t.Fatalf("suppress duration = %d, want 600", rec.Condition.SuppressDurationS)
	}
	if rec.ResourceID != "rds-instance-1" || rec.ResourceName != "prod-db" {
		t.Fatalf("resource = %q/%q", rec.ResourceID, rec.ResourceName)
	}
	if rec.Status != "alarm" || rec.Level != "2" {
		t.Fatalf("status/level = %q/%q", rec.Status, rec.Level)
	}
}

func TestMapAlarmHistoryWithoutCondition(t *testing.T) {
	item := cesv2model.AlarmHistoryItemV2{
		RecordId:  strPtr("ah-event-1"),
		Name:      strPtr("系统事件告警"),
		Condition: &cesv2model.AlarmHistoryItemV2Condition{},
	}

	rec := mapAlarmHistory(item, domain.AlarmPullScope{Region: "cn-north-4", ProjectID: "p1"})

	if rec.Condition != nil {
		t.Fatalf("empty condition should map to nil, got %+v", rec.Condition)
	}
}
