package huawei

import (
	"testing"

	"github.com/734965549/aiops/internal/observability/domain"
)

func TestMapMetricQuery(t *testing.T) {
	q := domain.MetricQuery{
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1710000000,
		To:         1710003600,
		Period:     60,
		Aggregator: "avg",
	}
	mapped, err := MapMetricQuery(q)
	if err != nil {
		t.Fatalf("MapMetricQuery: %v", err)
	}
	if mapped.Namespace != "SYS.ECS" || mapped.MetricName != "cpu_util" {
		t.Fatalf("unexpected mapped identity: %+v", mapped)
	}
	if mapped.FromMS != 1710000000000 || mapped.ToMS != 1710003600000 {
		t.Fatalf("unexpected time range: from=%d to=%d", mapped.FromMS, mapped.ToMS)
	}
	if mapped.Period != 60 {
		t.Fatalf("expected period snapped to 60, got %d", mapped.Period)
	}
	if mapped.Filter != "average" {
		t.Fatalf("expected average filter, got %q", mapped.Filter)
	}
	if len(mapped.Dimensions) != 1 || mapped.Dimensions[0].Name != "instance_id" {
		t.Fatalf("unexpected dimensions: %+v", mapped.Dimensions)
	}
}

func TestMapMetricDataResult(t *testing.T) {
	avg42 := 42.1
	q := domain.MetricQuery{
		Metric:     "cpu_util",
		Aggregator: "avg",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
	}
	result := MetricDataResult{
		MetricName: "cpu_util",
		Unit:       "%",
		Points: []MetricDataPoint{{
			TimestampMS: 1710000000000,
			Average:     &avg42,
			Unit:        "%",
		}},
	}
	series := MapMetricDataResult(q, result)
	if len(series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(series))
	}
	if series[0].Unit != "Percent" {
		t.Fatalf("expected Percent unit, got %q", series[0].Unit)
	}
	if len(series[0].Points) != 1 || series[0].Points[0].TS != 1710000000 {
		t.Fatalf("unexpected points: %+v", series[0].Points)
	}
	if series[0].Points[0].Value != 42.1 {
		t.Fatalf("unexpected value: %v", series[0].Points[0].Value)
	}
}

func TestMapMetricDataResultInfersUnit(t *testing.T) {
	value := 20480.0
	series := MapMetricDataResult(domain.MetricQuery{
		Metric:     "rds045_oldest_replication_slot_lag",
		Aggregator: "avg",
	}, MetricDataResult{
		MetricName: "rds045_oldest_replication_slot_lag",
		Points:     []MetricDataPoint{{TimestampMS: 1710000000000, Average: &value}},
	})
	if len(series) != 1 || series[0].Unit != "MiB" {
		t.Fatalf("expected inferred MiB unit, got %+v", series)
	}
}

func TestInferUnitFromMetric(t *testing.T) {
	cases := map[string]string{
		"rds039_disk_util":                   "Percent",
		"rds072_conn_usage":                  "Percent",
		"rds083_conn_usage":                  "Percent",
		"rds045_oldest_replication_slot_lag": "MiB",
		"rds073_replication_delay":           "s",
		"rds042_database_connections":        "Count",
		"oldest_transaction_duration":        "ms",
		// _usage 后缀不是百分比：复制插槽使用量是 MiB，不能按后缀臆造。
		"rds040_transaction_logs_usage": "",
		"unknown_metric":                "",
	}
	for metric, want := range cases {
		if got := inferUnitFromMetric(metric); got != want {
			t.Fatalf("inferUnitFromMetric(%q) = %q, want %q", metric, got, want)
		}
	}
}

func TestMapMetricQueryRejectsTooManyDimensions(t *testing.T) {
	dims := map[string]string{
		"d0": "v0", "d1": "v1", "d2": "v2", "d3": "v3", "d4": "v4",
	}
	_, err := MapMetricQuery(domain.MetricQuery{
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: dims,
		From:       1,
		To:         2,
	})
	if err == nil {
		t.Fatal("expected error for too many dimensions")
	}
}
