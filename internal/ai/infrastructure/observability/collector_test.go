package observability

import (
	"context"
	"errors"
	"testing"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

type fakeMetricsQuerier struct {
	result *obsapp.MetricQueryResult
	err    error
	called bool
	actor  obsapp.Actor
	query  obsdomain.MetricQuery
	block  bool
}

func (f *fakeMetricsQuerier) QueryMetrics(ctx context.Context, actor obsapp.Actor, q obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error) {
	f.called = true
	f.actor = actor
	f.query = q
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.result, f.err
}

type fakeAccountResolver struct {
	snapshot *obsdomain.AccountSnapshot
	err      error
}

func (f *fakeAccountResolver) ResolveAccount(context.Context, string) (*obsdomain.AccountSnapshot, error) {
	return f.snapshot, f.err
}

type fakeAccountAccess struct {
	err    error
	called bool
	ref    aiapp.AccountRef
}

func (f *fakeAccountAccess) Authorize(_ context.Context, _ string, ref aiapp.AccountRef) error {
	f.called = true
	f.ref = ref
	return f.err
}

func evidenceQuery() aiapp.MetricEvidenceQuery {
	threshold := 90.0
	return aiapp.MetricEvidenceQuery{
		AccountID: "acc-1", Region: "cn-north-4", ProjectID: "p-1",
		Namespace: "SYS.ECS", Metric: "cpu_util",
		Dimensions: map[string]string{"instance_id": "i-1"},
		From:       1700000000, To: 1700003600, Period: 300, Aggregator: "average",
		Threshold: &threshold, Operator: ">",
	}
}

func TestCollectorMapsMetricEvidence(t *testing.T) {
	querier := &fakeMetricsQuerier{result: &obsapp.MetricQueryResult{
		EvidenceID: "ev-1",
		Series: []obsdomain.MetricSeries{{
			Metric: "cpu_util", Unit: "%", Labels: map[string]string{"instance_id": "i-1"},
			Points: []obsdomain.MetricPoint{{TS: 100, Value: 10}, {TS: 200, Value: 120}, {TS: 300, Value: 95}},
		}},
	}}
	resolver := &fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{
		AccountID: "acc-1", AuthType: "ak_sk", OwnerTeam: "sre", Regions: []string{"cn-north-4"},
	}}
	access := &fakeAccountAccess{}
	collector := NewCollectorAdapter(querier, resolver, access, time.Second)

	result, err := collector.Collect(context.Background(), "user-1", evidenceQuery())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusOK || len(result.Metrics) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	metric := result.Metrics[0]
	if metric.EvidenceID != "ev-1" || metric.First == nil || *metric.First != 10 ||
		metric.Peak == nil || *metric.Peak != 120 || metric.Last == nil || *metric.Last != 95 {
		t.Fatalf("unexpected summary: %+v", metric)
	}
	if metric.BreachPoints != 2 {
		t.Fatalf("breach points = %d, want 2", metric.BreachPoints)
	}
	if len(metric.Points) != 3 || metric.Points[0][1] != 10 {
		t.Fatalf("unexpected points: %+v", metric.Points)
	}
	if !access.called || access.ref.OwnerTeam != "sre" || access.ref.Regions[0] != "cn-north-4" {
		t.Fatalf("unexpected access check: %+v", access)
	}
	if querier.actor.UserID != "user-1" || querier.query.AccountID != "acc-1" || querier.query.ProjectID != "p-1" {
		t.Fatalf("unexpected query call: actor=%+v query=%+v", querier.actor, querier.query)
	}
}

func TestCollectorSkipsMissingFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*aiapp.MetricEvidenceQuery)
		want   string
	}{
		{name: "account", mutate: func(q *aiapp.MetricEvidenceQuery) { q.AccountID = "" }, want: "no_account"},
		{name: "region", mutate: func(q *aiapp.MetricEvidenceQuery) { q.Region = "" }, want: "no_region"},
		{name: "namespace", mutate: func(q *aiapp.MetricEvidenceQuery) { q.Namespace = "" }, want: "no_namespace"},
		{name: "metric", mutate: func(q *aiapp.MetricEvidenceQuery) { q.Metric = "" }, want: "no_metric"},
		{name: "dimensions", mutate: func(q *aiapp.MetricEvidenceQuery) { q.Dimensions = nil }, want: "no_dimensions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := evidenceQuery()
			tc.mutate(&query)
			collector := NewCollectorAdapter(&fakeMetricsQuerier{}, &fakeAccountResolver{}, nil, time.Second)
			result, err := collector.Collect(context.Background(), "u", query)
			if err != nil {
				t.Fatalf("collect: %v", err)
			}
			if result.Status != aiapp.EvidenceStatusSkipped || result.Reason != tc.want {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestCollectorRejectsAgencyAuth(t *testing.T) {
	querier := &fakeMetricsQuerier{}
	collector := NewCollectorAdapter(querier, &fakeAccountResolver{
		snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", AuthType: "agency"},
	}, nil, time.Second)

	result, err := collector.Collect(context.Background(), "u", evidenceQuery())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusUnsupported {
		t.Fatalf("unexpected result: %+v", result)
	}
	if querier.called {
		t.Fatal("agency auth must not call metrics query")
	}
}

func TestCollectorPermissionDeniedDoesNotQuery(t *testing.T) {
	querier := &fakeMetricsQuerier{}
	collector := NewCollectorAdapter(
		querier,
		&fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk"}},
		&fakeAccountAccess{err: apperr.New(apperr.CodePermissionDenied, "denied")},
		time.Second,
	)

	result, err := collector.Collect(context.Background(), "u", evidenceQuery())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusPermissionDenied || querier.called {
		t.Fatalf("unexpected result/call: %+v called=%v", result, querier.called)
	}
}

func TestCollectorDegradesAccountAndQueryErrors(t *testing.T) {
	collector := NewCollectorAdapter(&fakeMetricsQuerier{}, &fakeAccountResolver{
		err: apperr.New(apperr.CodeNotFound, "integration account not found"),
	}, nil, time.Second)
	result, err := collector.Collect(context.Background(), "u", evidenceQuery())
	if err != nil {
		t.Fatalf("account error must not return error: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusError || result.Reason != "integration account not found" {
		t.Fatalf("unexpected account error result: %+v", result)
	}

	collector = NewCollectorAdapter(
		&fakeMetricsQuerier{err: errors.New("raw upstream detail")},
		&fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk"}},
		nil,
		time.Second,
	)
	result, err = collector.Collect(context.Background(), "u", evidenceQuery())
	if err != nil {
		t.Fatalf("query error must not return error: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusError || result.Reason != "metric query failed" {
		t.Fatalf("unexpected query error result: %+v", result)
	}
}

func TestCollectorTimesOut(t *testing.T) {
	collector := NewCollectorAdapter(
		&fakeMetricsQuerier{block: true},
		&fakeAccountResolver{snapshot: &obsdomain.AccountSnapshot{AccountID: "acc-1", AuthType: "ak_sk"}},
		nil,
		10*time.Millisecond,
	)
	result, err := collector.Collect(context.Background(), "u", evidenceQuery())
	if err != nil {
		t.Fatalf("timeout must not return error: %v", err)
	}
	if result.Status != aiapp.EvidenceStatusError || result.Reason != "metric query timeout" {
		t.Fatalf("unexpected timeout result: %+v", result)
	}
}

func TestCollectorRequiresQuerier(t *testing.T) {
	collector := NewCollectorAdapter(nil, &fakeAccountResolver{}, nil, time.Second)
	if _, err := collector.Collect(context.Background(), "u", evidenceQuery()); apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}
