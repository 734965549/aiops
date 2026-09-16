package huawei

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/734965549/aiops/internal/observability/domain"
)

// MetricDataQuery 归一化后的 CES ShowMetricData 请求参数（不含 SDK 类型）。
type MetricDataQuery struct {
	Namespace  string
	MetricName string
	Dimensions []MetricDimension
	FromMS     int64
	ToMS       int64
	Period     int32
	Filter     string
}

// MetricDimension CES 维度 name/value 对。
type MetricDimension struct {
	Name  string
	Value string
}

// MetricDataResult CES 指标查询结果（已从 SDK 响应归一化）。
type MetricDataResult struct {
	MetricName string
	Namespace  string
	Unit       string
	Points     []MetricDataPoint
}

// MetricDataPoint CES 单个采样点。
type MetricDataPoint struct {
	TimestampMS int64
	Average     *float64
	Max         *float64
	Min         *float64
	Sum         *float64
	Variance    *float64
	Unit        string
}

// cesAllowedPeriods CES 支持的聚合粒度（秒），见 ShowMetricData API 文档。
var cesAllowedPeriods = []int32{1, 60, 300, 1200, 3600, 14400, 86400}

// MapMetricQuery 将平台 domain.MetricQuery 映射为 CES 请求参数。
func MapMetricQuery(q domain.MetricQuery) (MetricDataQuery, error) {
	namespace := strings.TrimSpace(q.Namespace)
	metric := strings.TrimSpace(q.Metric)
	if namespace == "" || metric == "" {
		return MetricDataQuery{}, domain.ErrInvalidArgument
	}
	fromMS, toMS, err := mapTimeRange(q.From, q.To)
	if err != nil {
		return MetricDataQuery{}, err
	}
	period := normalizeCESPeriod(q.Period)
	filter, err := mapAggregator(q.Aggregator)
	if err != nil {
		return MetricDataQuery{}, err
	}
	dims, err := mapDimensions(q.Dimensions)
	if err != nil {
		return MetricDataQuery{}, err
	}
	return MetricDataQuery{
		Namespace:  namespace,
		MetricName: metric,
		Dimensions: dims,
		FromMS:     fromMS,
		ToMS:       toMS,
		Period:     period,
		Filter:     filter,
	}, nil
}

// MapMetricDataResult 将 CES 查询结果转换为平台 domain.MetricSeries。
func MapMetricDataResult(q domain.MetricQuery, result MetricDataResult) []domain.MetricSeries {
	metric := strings.TrimSpace(q.Metric)
	if metric == "" {
		metric = result.MetricName
	}
	filter := normalizeAggregator(q.Aggregator)
	labels := cloneStringMap(q.Dimensions)
	if labels == nil {
		labels = map[string]string{}
	}
	unit := normalizeUnit(result.Unit)
	points := make([]domain.MetricPoint, 0, len(result.Points))
	for _, dp := range result.Points {
		value, ok := pickDatapointValue(dp, filter)
		if !ok {
			continue
		}
		ts := dp.TimestampMS / 1000
		if dp.TimestampMS > 0 && dp.TimestampMS%1000 != 0 {
			ts = int64(math.Round(float64(dp.TimestampMS) / 1000.0))
		}
		points = append(points, domain.MetricPoint{TS: ts, Value: value})
		if unit == "" && dp.Unit != "" {
			unit = normalizeUnit(dp.Unit)
		}
	}
	if unit == "" {
		unit = inferUnitFromMetric(metric)
	}
	return []domain.MetricSeries{{
		Metric: metric,
		Unit:   unit,
		Labels: labels,
		Points: points,
	}}
}

func mapTimeRange(fromSec, toSec int64) (int64, int64, error) {
	if fromSec <= 0 || toSec <= 0 || toSec <= fromSec {
		return 0, 0, domain.ErrInvalidArgument
	}
	return fromSec * 1000, toSec * 1000, nil
}

func mapDimensions(dims map[string]string) ([]MetricDimension, error) {
	if len(dims) == 0 {
		return nil, nil
	}
	if len(dims) > 4 {
		return nil, fmt.Errorf("%w: at most 4 dimensions are supported", domain.ErrInvalidArgument)
	}
	keys := make([]string, 0, len(dims))
	for k, v := range dims {
		name := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if name == "" || value == "" {
			return nil, domain.ErrInvalidArgument
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	out := make([]MetricDimension, 0, len(keys))
	for _, name := range keys {
		out = append(out, MetricDimension{Name: name, Value: strings.TrimSpace(dims[name])})
	}
	return out, nil
}

func mapAggregator(aggregator string) (string, error) {
	switch normalizeAggregator(aggregator) {
	case "average":
		return "average", nil
	case "max":
		return "max", nil
	case "min":
		return "min", nil
	case "sum":
		return "sum", nil
	case "variance":
		return "variance", nil
	default:
		return "", domain.ErrInvalidArgument
	}
}

func normalizeAggregator(aggregator string) string {
	switch strings.ToLower(strings.TrimSpace(aggregator)) {
	case "", "avg", "average", "mean":
		return "average"
	case "max", "maximum":
		return "max"
	case "min", "minimum":
		return "min"
	case "sum", "total":
		return "sum"
	case "variance", "var":
		return "variance"
	default:
		return strings.ToLower(strings.TrimSpace(aggregator))
	}
}

func normalizeCESPeriod(period int) int32 {
	if period <= 0 {
		period = 60
	}
	p := int32(period)
	for _, allowed := range cesAllowedPeriods {
		if p <= allowed {
			return allowed
		}
	}
	return cesAllowedPeriods[len(cesAllowedPeriods)-1]
}

func pickDatapointValue(dp MetricDataPoint, filter string) (float64, bool) {
	switch filter {
	case "max":
		if dp.Max != nil {
			return *dp.Max, true
		}
	case "min":
		if dp.Min != nil {
			return *dp.Min, true
		}
	case "sum":
		if dp.Sum != nil {
			return *dp.Sum, true
		}
	case "variance":
		if dp.Variance != nil {
			return *dp.Variance, true
		}
	default:
		if dp.Average != nil {
			return *dp.Average, true
		}
	}
	if dp.Average != nil {
		return *dp.Average, true
	}
	if dp.Max != nil {
		return *dp.Max, true
	}
	if dp.Min != nil {
		return *dp.Min, true
	}
	if dp.Sum != nil {
		return *dp.Sum, true
	}
	if dp.Variance != nil {
		return *dp.Variance, true
	}
	return 0, false
}

func normalizeUnit(unit string) string {
	switch strings.TrimSpace(unit) {
	case "%", "percent", "Percent":
		return "Percent"
	default:
		return strings.TrimSpace(unit)
	}
}

func inferUnitFromMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "cpu_util", "cpu_usage", "mem_util", "memory_util", "disk_util":
		return "Percent"
	default:
		return ""
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
