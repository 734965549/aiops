package huawei

import (
	"fmt"
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
)

// validateCESMetricQuery 校验华为 CES 指标查询必填项（region/namespace/metric/project_id/dimensions）。
func validateCESMetricQuery(pctx domain.ProviderContext, q domain.MetricQuery) error {
	region := strings.TrimSpace(q.Region)
	if region == "" {
		return fmt.Errorf("%w: region is required", domain.ErrInvalidArgument)
	}
	if len(pctx.Account.Regions) > 0 && !containsRegion(pctx.Account.Regions, region) {
		return fmt.Errorf("%w: region is not configured for account", domain.ErrInvalidArgument)
	}
	if strings.TrimSpace(pctx.Account.ProjectID) == "" {
		return fmt.Errorf("%w: project_id is required", domain.ErrInvalidArgument)
	}
	if strings.TrimSpace(q.Namespace) == "" {
		return fmt.Errorf("%w: namespace is required", domain.ErrInvalidArgument)
	}
	if strings.TrimSpace(q.Metric) == "" {
		return fmt.Errorf("%w: metric is required", domain.ErrInvalidArgument)
	}
	if len(q.Dimensions) == 0 {
		return fmt.Errorf("%w: at least one dimension is required", domain.ErrInvalidArgument)
	}
	if len(q.Dimensions) > 4 {
		return fmt.Errorf("%w: at most 4 dimensions are supported", domain.ErrInvalidArgument)
	}
	for k, v := range q.Dimensions {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			return fmt.Errorf("%w: dimension name and value are required", domain.ErrInvalidArgument)
		}
	}
	return nil
}

func containsRegion(regions []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, r := range regions {
		if strings.TrimSpace(r) == want {
			return true
		}
	}
	return false
}
