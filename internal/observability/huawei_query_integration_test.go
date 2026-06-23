package observability_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	integdomain "github.com/734965549/aiops/internal/integration/domain"
	integcred "github.com/734965549/aiops/internal/integration/infrastructure/credential"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/internal/observability/infrastructure/provider/huawei"
)

// QueryService 经真实 huawei.Adapter（mock CES + vault 凭据）的集成式单测。

func TestQueryService_QueryMetricsHuaweiAdapterIntegration(t *testing.T) {
	avg := 33.3
	mockCES := &mockCESClient{result: huawei.MetricDataResult{
		MetricName: "cpu_util",
		Unit:       "%",
		Points: []huawei.MetricDataPoint{{
			TimestampMS: 1710000000000,
			Average:     &avg,
			Unit:        "%",
		}},
	}}
	credProvider, credRepo := newHuaweiCredentialProvider(t)
	const accountID = "acc-huawei-metrics"
	seedHuaweiAKSK(t, credRepo, accountID)

	adapter := huawei.NewAdapter(credProvider, mockCES, nil)
	svc := obsapp.NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID:       accountID,
		Provider:        string(integdomain.ProviderHuaweiCloud),
		AuthType:        string(integdomain.AuthAKSK),
		ProjectID:       "proj-1",
		Regions:         []string{"cn-north-4"},
		CredentialRefID: "cref-1",
		Capabilities:    []string{string(integdomain.CapabilityMetrics)},
	}}, stubRegistry{p: adapter}, &memEvidenceRepo{}, &captureAudit{})

	out, err := svc.QueryMetrics(context.Background(), obsapp.Actor{UserID: "u1"}, domain.MetricQuery{
		AccountID:  accountID,
		Provider:   string(integdomain.ProviderHuaweiCloud),
		Region:     "cn-north-4",
		Namespace:  "SYS.ECS",
		Metric:     "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1710000000,
		To:         1710003600,
		Period:     60,
		Aggregator: "avg",
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if out.EvidenceID == "" {
		t.Fatal("expected evidence_id")
	}
	if len(out.Series) != 1 || out.Series[0].Metric != "cpu_util" || out.Series[0].Points[0].Value != 33.3 {
		t.Fatalf("unexpected series: %+v", out.Series)
	}
	if mockCES.last.region != "cn-north-4" || mockCES.last.query.Namespace != "SYS.ECS" {
		t.Fatalf("adapter did not reach mock ces: %+v", mockCES.last)
	}
}

func TestQueryService_QueryMetricsHuaweiAdapterRedactsAudit(t *testing.T) {
	v := 1.0
	mockCES := &mockCESClient{result: huawei.MetricDataResult{
		MetricName: "cpu_util",
		Points:     []huawei.MetricDataPoint{{TimestampMS: 1710000000000, Average: &v}},
	}}
	credProvider, credRepo := newHuaweiCredentialProvider(t)
	const accountID = "acc-huawei-audit"
	seedHuaweiAKSK(t, credRepo, accountID)

	audit := &captureAudit{}
	evidence := &memEvidenceRepo{}
	adapter := huawei.NewAdapter(credProvider, mockCES, nil)
	svc := obsapp.NewQueryService(stubAccountPort{acc: &domain.AccountSnapshot{
		AccountID: accountID, Provider: string(integdomain.ProviderHuaweiCloud),
		AuthType: string(integdomain.AuthAKSK), ProjectID: "proj-1",
		Regions: []string{"cn-north-4"}, CredentialRefID: "cref-1",
		Capabilities: []string{string(integdomain.CapabilityMetrics)},
	}}, stubRegistry{p: adapter}, evidence, audit)

	_, err := svc.QueryMetrics(context.Background(), obsapp.Actor{UserID: "u1"}, domain.MetricQuery{
		AccountID: accountID, Provider: string(integdomain.ProviderHuaweiCloud),
		Region: "cn-north-4", Namespace: "SYS.ECS", Metric: "cpu_util",
		Dimensions: map[string]string{"instance_id": "ecs-1"},
		From:       1710000000, To: 1710003600,
	})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	rawAudit, _ := json.Marshal(audit.last.Payload)
	rawEvidence, _ := json.Marshal(evidence.refs[0].Summary)
	for _, blob := range []string{string(rawAudit), string(rawEvidence)} {
		if strings.Contains(blob, "SKTEST9876543210") || strings.Contains(blob, "AKTEST123456") {
			t.Fatalf("sensitive credential leaked: audit=%s evidence=%s", rawAudit, rawEvidence)
		}
	}
	if audit.last.Action != obsapp.AuditMetricsQuery {
		t.Fatalf("expected metrics_query audit, got %q", audit.last.Action)
	}
}

type mockCESClient struct {
	result huawei.MetricDataResult
	err    error
	last   struct {
		projectID string
		region    string
		query     huawei.MetricDataQuery
	}
}

func (m *mockCESClient) QueryMetricData(_ context.Context, _ huawei.AKSKCredential, projectID, region string, query huawei.MetricDataQuery) (huawei.MetricDataResult, error) {
	m.last.projectID = projectID
	m.last.region = region
	m.last.query = query
	if m.err != nil {
		return huawei.MetricDataResult{}, m.err
	}
	return m.result, nil
}

type stubAccountPort struct {
	acc *domain.AccountSnapshot
}

func (s stubAccountPort) ResolveAccount(context.Context, string) (*domain.AccountSnapshot, error) {
	return s.acc, nil
}

type memEvidenceRepo struct {
	refs []*domain.EvidenceRef
}

func (m *memEvidenceRepo) Create(_ context.Context, ref *domain.EvidenceRef) error {
	cp := *ref
	m.refs = append(m.refs, &cp)
	return nil
}

func (m *memEvidenceRepo) GetByID(context.Context, string) (*domain.EvidenceRef, error) {
	return nil, domain.ErrNotFound
}

type captureAudit struct {
	last obsapp.AuditRecord
}

func (c *captureAudit) Record(_ context.Context, rec obsapp.AuditRecord) error {
	c.last = rec
	return nil
}

type stubRegistry struct {
	p obsapp.ProviderEntry
}

func (r stubRegistry) Get(string) (obsapp.ProviderEntry, error) { return r.p, nil }

type huaweiMemCredentialRepo struct {
	byAccount map[string]*integdomain.CredentialRef
	vault     integdomain.CredentialVault
}

func (r *huaweiMemCredentialRepo) Create(_ context.Context, ref *integdomain.CredentialRef) error {
	r.byAccount[ref.AccountID] = ref
	return nil
}

func (r *huaweiMemCredentialRepo) Update(_ context.Context, ref *integdomain.CredentialRef) error {
	r.byAccount[ref.AccountID] = ref
	return nil
}

func (r *huaweiMemCredentialRepo) GetByAccountID(_ context.Context, accountID string) (*integdomain.CredentialRef, error) {
	ref, ok := r.byAccount[accountID]
	if !ok {
		return nil, integdomain.ErrNotFound
	}
	cp := *ref
	return &cp, nil
}

func (r *huaweiMemCredentialRepo) DeleteByAccountID(_ context.Context, accountID string) error {
	delete(r.byAccount, accountID)
	return nil
}

func newHuaweiCredentialProvider(t *testing.T) (*huawei.CredentialProvider, *huaweiMemCredentialRepo) {
	t.Helper()
	vault, err := integcred.NewVault("unit-test-huawei-query-key", 1)
	if err != nil {
		t.Fatalf("new vault: %v", err)
	}
	repo := &huaweiMemCredentialRepo{byAccount: map[string]*integdomain.CredentialRef{}, vault: vault}
	return huawei.NewCredentialProvider(repo, vault), repo
}

func seedHuaweiAKSK(t *testing.T, repo *huaweiMemCredentialRepo, accountID string) {
	t.Helper()
	material := integdomain.CredentialMaterial{
		"access_key": "AKTEST123456",
		"secret_key": "SKTEST9876543210",
	}
	ciphertext, _, err := repo.vault.Encrypt(material)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	repo.byAccount[accountID] = &integdomain.CredentialRef{
		AccountID: accountID, Ciphertext: ciphertext,
	}
}
