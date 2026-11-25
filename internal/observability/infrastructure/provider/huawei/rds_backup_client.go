package huawei

import (
	"context"
	"strings"

	apperr "github.com/734965549/aiops/pkg/errors"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
)

// RDSBackupClient 是 RDS 备份策略与备份记录只读接口。
type RDSBackupClient interface {
	ShowBackupPolicy(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string) (*rdsmodel.BackupPolicy, error)
	ListBackups(ctx context.Context, cred AKSKCredential, projectID, region, instanceID, begin, end string, limit int32) ([]rdsmodel.BackupForList, int32, error)
}

type rdsBackupClient struct{}

func NewRDSBackupClient() RDSBackupClient {
	return &rdsBackupClient{}
}

func (c *rdsBackupClient) ShowBackupPolicy(ctx context.Context, cred AKSKCredential, projectID, region, instanceID string) (*rdsmodel.BackupPolicy, error) {
	client, err := newHistoryRDSClient(ctx, cred, projectID, region)
	if err != nil {
		return nil, err
	}
	resp, err := callWithContext(ctx, func() (*rdsmodel.ShowBackupPolicyResponse, error) {
		return client.ShowBackupPolicy(&rdsmodel.ShowBackupPolicyRequest{
			InstanceId: strings.TrimSpace(instanceID),
		})
	})
	if err != nil {
		logResourceError(ctx, "rds-backup-policy", region, err)
		return nil, mapHuaweiAPIError(err)
	}
	if resp == nil || resp.BackupPolicy == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "rds backup policy returned empty response")
	}
	return resp.BackupPolicy, nil
}

func (c *rdsBackupClient) ListBackups(ctx context.Context, cred AKSKCredential, projectID, region, instanceID, begin, end string, limit int32) ([]rdsmodel.BackupForList, int32, error) {
	client, err := newHistoryRDSClient(ctx, cred, projectID, region)
	if err != nil {
		return nil, 0, err
	}
	limitCopy := limit
	resp, err := callWithContext(ctx, func() (*rdsmodel.ListBackupsResponse, error) {
		req := &rdsmodel.ListBackupsRequest{
			InstanceId: strings.TrimSpace(instanceID),
			Limit:      &limitCopy,
		}
		if begin = strings.TrimSpace(begin); begin != "" {
			req.BeginTime = &begin
		}
		if end = strings.TrimSpace(end); end != "" {
			req.EndTime = &end
		}
		return client.ListBackups(req)
	})
	if err != nil {
		logResourceError(ctx, "rds-backup-list", region, err)
		return nil, 0, mapHuaweiAPIError(err)
	}
	if resp == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "rds backup list returned empty response")
	}
	total := int32(0)
	if resp.TotalCount != nil {
		total = *resp.TotalCount
	}
	if resp.Backups == nil {
		return nil, total, nil
	}
	return *resp.Backups, total, nil
}
