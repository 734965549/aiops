package fake

import (
	"context"
	"testing"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/internal/observability/domain"
)

func TestProvider_QueryMetrics(t *testing.T) {
	p := New(string(integdomain.ProviderHuaweiCloud))
	series, err := p.QueryMetrics(context.Background(), domain.ProviderContext{
		Account: domain.AccountSnapshot{AccountID: "acc-1"},
	}, domain.MetricQuery{Metric: "cpu_util", From: 1710000000, To: 1710000120, Period: 60})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || len(series[0].Points) == 0 {
		t.Fatalf("unexpected series: %+v", series)
	}
}
