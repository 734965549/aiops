package huawei

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/sdkerr"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1/model"
)

const defaultCESTimeout = 30 * time.Second

// MetricDataClient CES 指标查询客户端；生产实现为 *CESClient，单测可注入 mock。
type MetricDataClient interface {
	QueryMetricData(ctx context.Context, cred AKSKCredential, projectID, region string, query MetricDataQuery) (MetricDataResult, error)
}

// CESClient 封装华为云 CES ShowMetricData API 调用。
type CESClient struct{}

var _ MetricDataClient = (*CESClient)(nil)

func NewCESClient() *CESClient {
	return &CESClient{}
}

// QueryMetricData 查询单条指标的时序数据。
func (c *CESClient) QueryMetricData(
	ctx context.Context,
	cred AKSKCredential,
	projectID, region string,
	query MetricDataQuery,
) (MetricDataResult, error) {
	if c == nil {
		return MetricDataResult{}, apperr.New(apperr.CodeUnavailable, "huawei ces client is not configured")
	}
	projectID = strings.TrimSpace(projectID)
	region = strings.TrimSpace(region)
	if projectID == "" || region == "" {
		return MetricDataResult{}, apperr.New(apperr.CodeInvalidArgument, "project_id and region are required")
	}
	if strings.TrimSpace(cred.AccessKey) == "" || strings.TrimSpace(cred.SecretKey) == "" {
		return MetricDataResult{}, apperr.New(apperr.CodeFailedPrecondition, "huawei ak/sk credential is required")
	}
	if err := ctx.Err(); err != nil {
		return MetricDataResult{}, mapCESError(err)
	}

	client, err := newCESClient(cred, projectID, region)
	if err != nil {
		return MetricDataResult{}, err
	}
	request, err := toShowMetricDataRequest(query)
	if err != nil {
		return MetricDataResult{}, err
	}

	response, err := client.ShowMetricData(request)
	if err != nil {
		mapped := mapCESError(err)
		fields := []logger.Field{
			logger.String("region", region),
			logger.String("namespace", query.Namespace),
			logger.String("metric", query.MetricName),
			logger.String("error_code", string(apperr.CodeOf(mapped))),
		}
		var svcErr *sdkerr.ServiceResponseError
		if errors.As(err, &svcErr) {
			fields = append(fields,
				logger.Int("huawei_status", svcErr.StatusCode),
				logger.String("huawei_error_code", strings.TrimSpace(svcErr.ErrorCode)),
			)
		}
		logger.From(ctx).Warn("huawei ces show metric data failed", fields...)
		return MetricDataResult{}, mapped
	}
	return fromShowMetricDataResponse(query, response), nil
}

func newCESClient(cred AKSKCredential, projectID, region string) (*ces.CesClient, error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(cred.AccessKey).
		WithSk(cred.SecretKey).
		WithProjectId(projectID).
		Build()
	endpoint := fmt.Sprintf("https://ces.%s.myhuaweicloud.com", region)
	client := ces.NewCesClient(
		ces.CesClientBuilder().
			WithEndpoints([]string{endpoint}).
			WithHttpConfig(config.DefaultHttpConfig().WithTimeout(defaultCESTimeout)).
			WithCredential(auth).
			Build(),
	)
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei ces client failed")
	}
	return client, nil
}

func toShowMetricDataRequest(query MetricDataQuery) (*model.ShowMetricDataRequest, error) {
	filter, err := toCESFilterEnum(query.Filter)
	if err != nil {
		return nil, err
	}
	period, err := toCESPeriodEnum(query.Period)
	if err != nil {
		return nil, err
	}
	req := &model.ShowMetricDataRequest{
		Namespace:  query.Namespace,
		MetricName: query.MetricName,
		From:       query.FromMS,
		To:         query.ToMS,
		Period:     period,
		Filter:     filter,
	}
	if len(query.Dimensions) > 0 {
		req.Dim0 = formatDimension(query.Dimensions[0])
	}
	if len(query.Dimensions) > 1 {
		dim1 := formatDimension(query.Dimensions[1])
		req.Dim1 = &dim1
	}
	if len(query.Dimensions) > 2 {
		dim2 := formatDimension(query.Dimensions[2])
		req.Dim2 = &dim2
	}
	if len(query.Dimensions) > 3 {
		dim3 := formatDimension(query.Dimensions[3])
		req.Dim3 = &dim3
	}
	return req, nil
}

func formatDimension(dim MetricDimension) string {
	return strings.TrimSpace(dim.Name) + "," + strings.TrimSpace(dim.Value)
}

func toCESPeriodEnum(period int32) (model.ShowMetricDataRequestPeriod, error) {
	switch period {
	case 1:
		return model.GetShowMetricDataRequestPeriodEnum().E_1, nil
	case 60:
		return model.GetShowMetricDataRequestPeriodEnum().E_60, nil
	case 300:
		return model.GetShowMetricDataRequestPeriodEnum().E_300, nil
	case 1200:
		return model.GetShowMetricDataRequestPeriodEnum().E_1200, nil
	case 3600:
		return model.GetShowMetricDataRequestPeriodEnum().E_3600, nil
	case 14400:
		return model.GetShowMetricDataRequestPeriodEnum().E_14400, nil
	case 86400:
		return model.GetShowMetricDataRequestPeriodEnum().E_86400, nil
	default:
		return model.ShowMetricDataRequestPeriod{}, apperr.New(apperr.CodeInvalidArgument, "unsupported ces period")
	}
}

func toCESFilterEnum(filter string) (model.ShowMetricDataRequestFilter, error) {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "average", "avg":
		return model.GetShowMetricDataRequestFilterEnum().AVERAGE, nil
	case "max":
		return model.GetShowMetricDataRequestFilterEnum().MAX, nil
	case "min":
		return model.GetShowMetricDataRequestFilterEnum().MIN, nil
	case "sum":
		return model.GetShowMetricDataRequestFilterEnum().SUM, nil
	case "variance", "var":
		return model.GetShowMetricDataRequestFilterEnum().VARIANCE, nil
	default:
		return model.ShowMetricDataRequestFilter{}, apperr.New(apperr.CodeInvalidArgument, "unsupported ces filter")
	}
}

func fromShowMetricDataResponse(query MetricDataQuery, resp *model.ShowMetricDataResponse) MetricDataResult {
	result := MetricDataResult{
		Namespace:  query.Namespace,
		MetricName: query.MetricName,
	}
	if resp == nil {
		return result
	}
	if resp.MetricName != nil && strings.TrimSpace(*resp.MetricName) != "" {
		result.MetricName = strings.TrimSpace(*resp.MetricName)
	}
	if resp.Datapoints == nil {
		return result
	}
	result.Points = make([]MetricDataPoint, 0, len(*resp.Datapoints))
	for _, dp := range *resp.Datapoints {
		point := MetricDataPoint{TimestampMS: dp.Timestamp}
		if dp.Average != nil {
			v := *dp.Average
			point.Average = &v
		}
		if dp.Max != nil {
			v := *dp.Max
			point.Max = &v
		}
		if dp.Min != nil {
			v := *dp.Min
			point.Min = &v
		}
		if dp.Sum != nil {
			v := *dp.Sum
			point.Sum = &v
		}
		if dp.Variance != nil {
			v := *dp.Variance
			point.Variance = &v
		}
		if dp.Unit != nil {
			point.Unit = strings.TrimSpace(*dp.Unit)
		}
		result.Points = append(result.Points, point)
		if result.Unit == "" && point.Unit != "" {
			result.Unit = point.Unit
		}
	}
	return result
}
