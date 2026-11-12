// Package observability 将 Observability 查询能力适配为 AI 指标证据端口。
package observability

import (
	"context"
	"errors"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	obsapp "github.com/734965549/aiops/internal/observability/application"
	obsdomain "github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const defaultEvidenceTimeout = 5 * time.Second

// MetricsQuerier 是 CollectorAdapter 依赖的只读指标查询能力（由 obsapp.QueryService 实现）。
type MetricsQuerier interface {
	QueryMetrics(ctx context.Context, actor obsapp.Actor, q obsdomain.MetricQuery) (*obsapp.MetricQueryResult, error)
}

// AccountResolver 解析 integration 账号摘要，用于认证方式与数据权限预检。
type AccountResolver interface {
	ResolveAccount(ctx context.Context, accountID string) (*obsdomain.AccountSnapshot, error)
}

// CollectorAdapter 实现 aiapp.MetricEvidencePort。
type CollectorAdapter struct {
	queries       MetricsQuerier
	accounts      AccountResolver
	accountAccess aiapp.AccountAccessAuthorizer
	timeout       time.Duration
}

func NewCollectorAdapter(
	queries MetricsQuerier,
	accounts AccountResolver,
	accountAccess aiapp.AccountAccessAuthorizer,
	timeout time.Duration,
) *CollectorAdapter {
	if timeout <= 0 {
		timeout = defaultEvidenceTimeout
	}
	return &CollectorAdapter{
		queries:       queries,
		accounts:      accounts,
		accountAccess: accountAccess,
		timeout:       timeout,
	}
}

var _ aiapp.MetricEvidencePort = (*CollectorAdapter)(nil)

// Collect 执行账号/权限预检后调用 Observation QueryService 取数。
// 除端口本身未装配外，所有业务失败都以 Result.Status 降级返回，保证分析编排不中断。
func (a *CollectorAdapter) Collect(ctx context.Context, userID string, q aiapp.MetricEvidenceQuery) (*aiapp.MetricEvidenceResult, error) {
	if a == nil || a.queries == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai metric evidence collector is not configured")
	}
	q.AccountID = strings.TrimSpace(q.AccountID)
	q.Region = strings.TrimSpace(q.Region)
	q.ProjectID = strings.TrimSpace(q.ProjectID)
	q.Namespace = strings.TrimSpace(q.Namespace)
	q.Metric = strings.TrimSpace(q.Metric)
	switch {
	case q.AccountID == "":
		return skippedEvidence("no_account"), nil
	case q.Region == "":
		return skippedEvidence("no_region"), nil
	case q.Namespace == "":
		return skippedEvidence("no_namespace"), nil
	case q.Metric == "":
		return skippedEvidence("no_metric"), nil
	case len(q.Dimensions) == 0:
		return skippedEvidence("no_dimensions"), nil
	}
	if a.accounts == nil {
		return degradedEvidence(aiapp.EvidenceStatusError, "account_resolver_unavailable"), nil
	}
	snapshot, err := a.accounts.ResolveAccount(ctx, q.AccountID)
	if err != nil {
		return degradedEvidence(aiapp.EvidenceStatusError, evidenceErrorReason(err, "resolve integration account failed")), nil
	}
	if snapshot == nil {
		return degradedEvidence(aiapp.EvidenceStatusError, "integration account not found"), nil
	}
	if strings.EqualFold(strings.TrimSpace(snapshot.AuthType), "agency") {
		return degradedEvidence(aiapp.EvidenceStatusUnsupported, "agency auth does not support metrics query"), nil
	}
	if a.accountAccess != nil && strings.TrimSpace(userID) != "" {
		ref := aiapp.AccountRef{
			AccountID: snapshot.AccountID,
			OwnerTeam: snapshot.OwnerTeam,
			Regions:   append([]string(nil), snapshot.Regions...),
		}
		if err := a.accountAccess.Authorize(ctx, userID, ref); err != nil {
			return degradedEvidence(aiapp.EvidenceStatusPermissionDenied, "no access to integration account for evidence"), nil
		}
	}

	queryCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	result, err := a.queries.QueryMetrics(queryCtx, obsapp.Actor{UserID: userID}, obsdomain.MetricQuery{
		AccountID:  q.AccountID,
		Region:     q.Region,
		ProjectID:  q.ProjectID,
		Namespace:  q.Namespace,
		Metric:     q.Metric,
		Dimensions: q.Dimensions,
		From:       q.From,
		To:         q.To,
		Period:     q.Period,
		Aggregator: q.Aggregator,
	})
	if err != nil {
		reason := evidenceErrorReason(err, "metric query failed")
		if errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			reason = "metric query timeout"
		}
		return degradedEvidence(aiapp.EvidenceStatusError, reason), nil
	}
	return mapMetricEvidence(q, result), nil
}

func skippedEvidence(reason string) *aiapp.MetricEvidenceResult {
	return &aiapp.MetricEvidenceResult{Status: aiapp.EvidenceStatusSkipped, Reason: reason}
}

func degradedEvidence(status, reason string) *aiapp.MetricEvidenceResult {
	return &aiapp.MetricEvidenceResult{Status: status, Reason: reason}
}

func evidenceErrorReason(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	ae := apperr.FromError(err)
	if ae == nil || ae.Code == apperr.CodeInternal {
		return fallback
	}
	if message := strings.TrimSpace(ae.Message); message != "" {
		return message
	}
	return fallback
}

func mapMetricEvidence(q aiapp.MetricEvidenceQuery, result *obsapp.MetricQueryResult) *aiapp.MetricEvidenceResult {
	out := &aiapp.MetricEvidenceResult{Status: aiapp.EvidenceStatusOK}
	if result == nil {
		return out
	}
	for _, series := range result.Series {
		metric := strings.TrimSpace(series.Metric)
		if metric == "" {
			metric = q.Metric
		}
		item := aiapp.MetricEvidence{
			Namespace:  q.Namespace,
			Metric:     metric,
			Unit:       series.Unit,
			Threshold:  q.Threshold,
			Operator:   q.Operator,
			Labels:     series.Labels,
			EvidenceID: result.EvidenceID,
		}
		if len(series.Points) > 0 {
			first := series.Points[0].Value
			last := series.Points[len(series.Points)-1].Value
			peak := first
			item.Points = make([][2]float64, 0, len(series.Points))
			for _, point := range series.Points {
				if point.Value > peak {
					peak = point.Value
				}
				if isBreachPoint(q.Operator, point.Value, q.Threshold) {
					item.BreachPoints++
				}
				item.Points = append(item.Points, [2]float64{float64(point.TS), point.Value})
			}
			item.First = &first
			item.Peak = &peak
			item.Last = &last
		}
		out.Metrics = append(out.Metrics, item)
	}
	return out
}

func isBreachPoint(operator string, value float64, threshold *float64) bool {
	if threshold == nil {
		return false
	}
	switch strings.TrimSpace(operator) {
	case ">":
		return value > *threshold
	case ">=":
		return value >= *threshold
	case "<":
		return value < *threshold
	case "<=":
		return value <= *threshold
	case "=", "==":
		return value == *threshold
	case "!=":
		return value != *threshold
	default:
		return false
	}
}
