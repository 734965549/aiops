package huawei

import (
	"context"
	"errors"
	"testing"

	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

type mockCESClient struct {
	result MetricDataResult
	err    error
	last   struct {
		cred      AKSKCredential
		projectID string
		region    string
		query     MetricDataQuery
	}
}

func (m *mockCESClient) QueryMetricData(_ context.Context, cred AKSKCredential, projectID, region string, query MetricDataQuery) (MetricDataResult, error) {
	m.last.cred = cred
	m.last.projectID = projectID
	m.last.region = region
	m.last.query = query
	if m.err != nil {
		return MetricDataResult{}, m.err
	}
	return m.result, nil
}

func TestCESClientQueryMetricDataRequiresProjectAndRegion(t *testing.T) {
	client := NewCESClient()
	_, err := client.QueryMetricData(context.Background(), AKSKCredential{
		AccessKey: "ak", SecretKey: "sk",
	}, "", "cn-north-4", MetricDataQuery{})
	if apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestCESClientQueryMetricDataRequiresCredential(t *testing.T) {
	client := NewCESClient()
	_, err := client.QueryMetricData(context.Background(), AKSKCredential{}, "proj-1", "cn-north-4", MetricDataQuery{})
	if apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestCESClientQueryMetricDataRespectsCanceledContext(t *testing.T) {
	client := NewCESClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.QueryMetricData(ctx, AKSKCredential{
		AccessKey: "ak", SecretKey: "sk",
	}, "proj-1", "cn-north-4", MetricDataQuery{})
	if !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable sentinel, got %v", err)
	}
}

func TestToShowMetricDataRequest(t *testing.T) {
	req, err := toShowMetricDataRequest(MetricDataQuery{
		Namespace:  "SYS.ECS",
		MetricName: "cpu_util",
		Dimensions: []MetricDimension{{Name: "instance_id", Value: "ecs-1"}},
		FromMS:     1710000000000,
		ToMS:       1710003600000,
		Period:     60,
		Filter:     "average",
	})
	if err != nil {
		t.Fatalf("toShowMetricDataRequest: %v", err)
	}
	if req.Namespace != "SYS.ECS" || req.MetricName != "cpu_util" {
		t.Fatalf("unexpected request identity: %+v", req)
	}
	if req.Dim0 != "instance_id,ecs-1" {
		t.Fatalf("unexpected dim0: %q", req.Dim0)
	}
}

func TestFromShowMetricDataResponse(t *testing.T) {
	avg := 55.5
	unit := "%"
	metricName := "cpu_util"
	resp := &model.ShowMetricDataResponse{
		MetricName: &metricName,
		Datapoints: &[]model.Datapoint{{
			Timestamp: 1710000000000,
			Average:   &avg,
			Unit:      &unit,
		}},
	}
	result := fromShowMetricDataResponse(MetricDataQuery{MetricName: "cpu_util"}, resp)
	if len(result.Points) != 1 || result.Points[0].Average == nil || *result.Points[0].Average != 55.5 {
		t.Fatalf("unexpected points: %+v", result.Points)
	}
	if result.Unit != "%" {
		t.Fatalf("expected unit %%, got %q", result.Unit)
	}
}

func TestAdapterQueryMetricsWithMockCESClient(t *testing.T) {
	avg := 77.7
	mockCES := &mockCESClient{result: MetricDataResult{
		MetricName: "cpu_util",
		Unit:       "%",
		Points:     []MetricDataPoint{{TimestampMS: 1710000000000, Average: &avg, Unit: "%"}},
	}}
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-ces"
	seedAKSKCredential(t, provider, repo, vault, accountID)

	adapter := NewAdapter(provider, mockCES, nil)
	series, err := adapter.QueryMetrics(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID:       accountID,
			AuthType:        "ak_sk",
			ProjectID:       "proj-1",
			Regions:         []string{"cn-north-4"},
			CredentialRefID: "cref-1",
		},
	}, domain.MetricQuery{
		Region: "cn-north-4", Namespace: "SYS.ECS", Metric: "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1710000000, To: 1710003600, Period: 60, Aggregator: "avg",
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 77.7 {
		t.Fatalf("unexpected series: %+v", series)
	}
	if mockCES.last.projectID != "proj-1" || mockCES.last.region != "cn-north-4" {
		t.Fatalf("unexpected ces call target: %+v", mockCES.last)
	}
	if mockCES.last.query.Namespace != "SYS.ECS" || mockCES.last.query.MetricName != "cpu_util" {
		t.Fatalf("unexpected ces query: %+v", mockCES.last.query)
	}
}

func TestAdapterQueryMetricsMockCESErrorPropagates(t *testing.T) {
	mockCES := &mockCESClient{err: domain.ErrProviderUnavailable}
	provider, repo, vault := newTestCredentialProvider(t)
	const accountID = "acc-ces-err"
	seedAKSKCredential(t, provider, repo, vault, accountID)

	adapter := NewAdapter(provider, mockCES, nil)
	_, err := adapter.QueryMetrics(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{
			AccountID: accountID, AuthType: "ak_sk", ProjectID: "proj-1",
			Regions: []string{"cn-north-4"}, CredentialRefID: "cref-1",
		},
	}, domain.MetricQuery{
		Region: "cn-north-4", Namespace: "SYS.ECS", Metric: "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1, To: 2,
	})
	if !errors.Is(err, domain.ErrProviderUnavailable) {
		t.Fatalf("expected provider unavailable, got %v", err)
	}
}
