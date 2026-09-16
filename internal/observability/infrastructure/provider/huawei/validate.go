package huawei

import (
	"fmt"
	"net/url"
	"strings"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// huaweiRegionMaxLen 限制 region 长度上限，避免超长输入造成额外开销。
const huaweiRegionMaxLen = 64

// validateRegion 校验华为云 region 格式，防止通过 region 注入路径/查询/主机分隔符
// 造成 SSRF 或签名请求外发。华为云 region 形如 cn-north-4、ap-southeast-1，
// 仅允许小写字母、数字与连字符。
func validateRegion(region string) error {
	region = strings.TrimSpace(region)
	if region == "" {
		return apperr.New(apperr.CodeInvalidArgument, "region is required")
	}
	if len(region) > huaweiRegionMaxLen {
		return apperr.New(apperr.CodeInvalidArgument, "region is too long")
	}
	for _, r := range region {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return apperr.New(apperr.CodeInvalidArgument, "region contains invalid characters")
		}
	}
	if strings.HasPrefix(region, "-") || strings.HasSuffix(region, "-") || strings.Contains(region, "--") {
		return apperr.New(apperr.CodeInvalidArgument, "region has invalid hyphen placement")
	}
	return nil
}

// buildEndpoint 构造华为云服务 endpoint 并对最终 URL 做安全复核，
// 确保 hostname 仍归属 myhuaweicloud.com，且不含 userinfo/path/query/fragment。
// service 形如 ces/ecs/rds 等，仅允许小写字母。
func buildEndpoint(service, region string) (string, error) {
	service = strings.TrimSpace(service)
	if service == "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "service is required")
	}
	for _, r := range service {
		if !(r >= 'a' && r <= 'z') {
			return "", apperr.New(apperr.CodeInvalidArgument, "service contains invalid characters")
		}
	}
	if err := validateRegion(region); err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("https://%s.%s.myhuaweicloud.com", service, region)
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid huawei endpoint")
	}
	if u.Scheme != "https" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid huawei endpoint")
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, ".myhuaweicloud.com") {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid huawei endpoint")
	}
	prefix := strings.TrimSuffix(host, ".myhuaweicloud.com")
	parts := strings.Split(prefix, ".")
	if len(parts) != 2 || parts[0] != service || parts[1] != region {
		return "", apperr.New(apperr.CodeInvalidArgument, "invalid huawei endpoint")
	}
	return endpoint, nil
}

// validateCESMetricQuery 校验华为 CES 指标查询必填项（region/namespace/metric/project_id/dimensions）。
// project_id 支持通过 region_projects 按 region 解析，只要最终解析结果非空即可，
// 不强制要求顶层 Account.ProjectID，见 ops/huawei-ces-sync-contract.md §5.3。
func validateCESMetricQuery(pctx domain.ProviderContext, q domain.MetricQuery) error {
	region := strings.TrimSpace(q.Region)
	if region == "" {
		return fmt.Errorf("%w: region is required", domain.ErrInvalidArgument)
	}
	if len(pctx.Account.Regions) > 0 && !containsRegion(pctx.Account.Regions, region) {
		return fmt.Errorf("%w: region is not configured for account", domain.ErrInvalidArgument)
	}
	cfg := obsapp.ParseSyncModeConfig(pctx.Account.ExtraConfig)
	if strings.TrimSpace(cfg.ResolveProjectID(region, pctx.Account.ProjectID)) == "" {
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
