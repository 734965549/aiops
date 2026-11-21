package huawei

import (
	"context"
	"strings"
	"time"

	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	rds "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

// RDSHistoryClient 是 RDS for PostgreSQL 历史会话分析只读接口。
type RDSHistoryClient interface {
	ListSessions(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, limit int32) ([]rdsmodel.Session, int32, error)
	ListWaitEvents(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, limit int32) ([]rdsmodel.Event, int32, error)
	ListTopSQL(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, limit int32) ([]rdsmodel.TopSql, int32, error)
}

type rdsHistoryClient struct{}

func NewRDSHistoryClient() RDSHistoryClient {
	return &rdsHistoryClient{}
}

func (c *rdsHistoryClient) ListSessions(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, limit int32) ([]rdsmodel.Session, int32, error) {
	client, err := newHistoryRDSClient(ctx, cred, projectID, region)
	if err != nil {
		return nil, 0, err
	}
	resp, err := callWithContext(ctx, func() (*rdsmodel.ListHistorySessionsResponse, error) {
		return client.ListHistorySessions(&rdsmodel.ListHistorySessionsRequest{
			InstanceId: strings.TrimSpace(instanceID), StartTime: startMS, EndTime: endMS,
			Offset: &offset, Limit: &limit,
		})
	})
	if err != nil {
		logResourceError(ctx, "rds-history-sessions", region, err)
		return nil, 0, mapHuaweiAPIError(err)
	}
	if resp == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "rds history sessions returned empty response")
	}
	total := historyTotalCount(resp.TotalCount)
	if resp.Sessions == nil {
		return nil, total, nil
	}
	return *resp.Sessions, total, nil
}

func (c *rdsHistoryClient) ListWaitEvents(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, limit int32) ([]rdsmodel.Event, int32, error) {
	client, err := newHistoryRDSClient(ctx, cred, projectID, region)
	if err != nil {
		return nil, 0, err
	}
	resp, err := callWithContext(ctx, func() (*rdsmodel.ListHistoryWaitEventsResponse, error) {
		return client.ListHistoryWaitEvents(&rdsmodel.ListHistoryWaitEventsRequest{
			InstanceId: strings.TrimSpace(instanceID), StartTime: startMS, EndTime: endMS,
			Offset: &offset, Limit: &limit,
		})
	})
	if err != nil {
		logResourceError(ctx, "rds-history-wait-events", region, err)
		return nil, 0, mapHuaweiAPIError(err)
	}
	if resp == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "rds history wait events returned empty response")
	}
	total := historyTotalCount(resp.TotalCount)
	if resp.Events == nil {
		return nil, total, nil
	}
	return *resp.Events, total, nil
}

func (c *rdsHistoryClient) ListTopSQL(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string, startMS, endMS int64, offset, limit int32) ([]rdsmodel.TopSql, int32, error) {
	client, err := newHistoryRDSClient(ctx, cred, projectID, region)
	if err != nil {
		return nil, 0, err
	}
	resp, err := callWithContext(ctx, func() (*rdsmodel.ListHistoryTopSqlsResponse, error) {
		return client.ListHistoryTopSqls(&rdsmodel.ListHistoryTopSqlsRequest{
			InstanceId: strings.TrimSpace(instanceID), StartTime: startMS, EndTime: endMS,
			Offset: &offset, Limit: &limit,
		})
	})
	if err != nil {
		logResourceError(ctx, "rds-history-top-sql", region, err)
		return nil, 0, mapHuaweiAPIError(err)
	}
	if resp == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "rds history top sql returned empty response")
	}
	total := historyTotalCount(resp.TotalCount)
	if resp.TopSqls == nil {
		return nil, total, nil
	}
	return *resp.TopSqls, total, nil
}

func historyTotalCount(total *int32) int32 {
	if total == nil {
		return -1
	}
	return *total
}

func newHistoryRDSClient(ctx context.Context, cred AKSKCredential, projectID, region string) (*rds.RdsClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := defaultResourceTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	auth := basic.NewCredentialsBuilder().WithAk(cred.AccessKey).WithSk(cred.SecretKey).WithProjectId(projectID).Build()
	endpoint, err := buildEndpoint("rds", region)
	if err != nil {
		return nil, err
	}
	client := rds.NewRdsClient(rds.RdsClientBuilder().
		WithEndpoints([]string{endpoint}).
		WithHttpConfig(config.DefaultHttpConfig().WithTimeout(timeout)).
		WithCredential(auth).
		Build())
	if client == nil {
		return nil, apperr.New(apperr.CodeInternal, "create huawei rds history client failed")
	}
	return client, nil
}
