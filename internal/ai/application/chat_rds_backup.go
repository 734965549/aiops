package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/logger"
)

var rdsBackupInstanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,128}$`)

type chatRDSBackupParams struct {
	AccountID  string `json:"account_id"`
	Region     string `json:"region"`
	ProjectID  string `json:"project_id"`
	InstanceID string `json:"instance_id"`
	Days       int    `json:"days"`
	Limit      int    `json:"limit"`
}

// buildRDSBackupQuery 解析备份查询参数：账号/区域/实例 ID 依次回退告警标签与资源上下文。
// 实例 ID 优先取 rds_instance_id，其次 rds_cluster_id，再其次 cloud_resource_id；
// 后两者在标准 RDS 实例上通常与实例 ID 一致，集群版对不上时由端口返回稳定错误。
func buildRDSBackupQuery(alert *AlertContext, resource *AssetContext, raw map[string]any, timezone string) (RDSBackupQuery, string, string) {
	params := chatRDSBackupParams{}
	if _, err := decodeToolParams(raw, &params); err != nil {
		return RDSBackupQuery{}, chatObservationStatusInvalidArgument, "invalid rds.backup.query params"
	}
	query := RDSBackupQuery{
		AccountID:  strings.TrimSpace(params.AccountID),
		Region:     strings.TrimSpace(params.Region),
		ProjectID:  strings.TrimSpace(params.ProjectID),
		InstanceID: strings.TrimSpace(params.InstanceID),
		Days:       params.Days,
		Limit:      params.Limit,
		Timezone:   strings.TrimSpace(timezone),
	}
	var labels map[string]string
	if alert != nil {
		labels = alert.Labels
	}
	if query.AccountID == "" {
		query.AccountID = strings.TrimSpace(labels["account_id"])
	}
	if query.AccountID == "" && resource != nil {
		query.AccountID = strings.TrimSpace(resource.AccountID)
	}
	if query.Region == "" {
		query.Region = strings.TrimSpace(labels["region"])
	}
	if query.Region == "" && resource != nil {
		query.Region = strings.TrimSpace(resource.Region)
	}
	if query.ProjectID == "" {
		query.ProjectID = strings.TrimSpace(labels["project_id"])
	}
	if query.ProjectID == "" && resource != nil && resource.Labels != nil {
		query.ProjectID = strings.TrimSpace(resource.Labels["project_id"])
	}
	if query.InstanceID == "" {
		for _, key := range []string{"rds_instance_id", "rds_cluster_id", "cloud_resource_id"} {
			if value := strings.TrimSpace(labels[key]); value != "" {
				query.InstanceID = value
				break
			}
		}
	}
	if query.InstanceID == "" && resource != nil {
		for _, key := range []string{"rds_instance_id", "rds_cluster_id"} {
			if resource.Labels != nil {
				if value := strings.TrimSpace(resource.Labels[key]); value != "" {
					query.InstanceID = value
					break
				}
			}
		}
		if query.InstanceID == "" {
			query.InstanceID = strings.TrimSpace(resource.CloudResourceID)
		}
	}
	if query.AccountID == "" {
		return query, EvidenceStatusSkipped, "no_account"
	}
	if query.Region == "" {
		return query, EvidenceStatusSkipped, "no_region"
	}
	if query.InstanceID == "" {
		return query, EvidenceStatusSkipped, "no_instance_id"
	}
	if !rdsBackupInstanceIDPattern.MatchString(query.InstanceID) {
		return query, chatObservationStatusInvalidArgument, "invalid rds backup instance_id"
	}
	if query.Days <= 0 {
		query.Days = RDSBackupDefaultDays
	}
	if query.Days > RDSBackupMaxDays {
		return query, chatObservationStatusInvalidArgument, "backup days must be at most 30"
	}
	if query.Limit <= 0 {
		query.Limit = RDSBackupDefaultLimit
	}
	if query.Limit > RDSBackupMaxLimit {
		return query, chatObservationStatusInvalidArgument, "backup limit must be at most 20"
	}
	return query, "", ""
}

// refreshRDSBackupObservation 查询 RDS 备份策略与最近备份记录（只读）。
func (s *ChatService) refreshRDSBackupObservation(ctx context.Context, userID string, session *domain.Session, data *chatContextData, raw map[string]any) bool {
	obs := s.checkTool(ctx, userID, chatToolRDSBackupQuery)
	if !obs.Allowed {
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if s.rdsBackups == nil {
		obs.Status = EvidenceStatusSkipped
		obs.Reason = "rds backup collector is not configured"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	query, status, reason := buildRDSBackupQuery(data.Alert, data.Resource, raw, s.timezone)
	if status != "" {
		obs.Status = status
		obs.Reason = reason
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	result, err := s.rdsBackups.Query(ctx, userID, query)
	if err != nil {
		logger.From(ctx).Warn("ai rds backup query failed", logger.Error(err))
		obs.Status = EvidenceStatusError
		obs.Reason = "rds backup query failed"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	if result == nil {
		obs.Status = EvidenceStatusError
		obs.Reason = "empty rds backup result"
		s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
		return false
	}
	obs.Status = result.Status
	if obs.Status == "" {
		obs.Status = EvidenceStatusOK
	}
	obs.Reason = result.Reason
	obs.Summary = rdsBackupSummary(result)
	obs.Data = map[string]any{
		"status": obs.Status, "reason": obs.Reason,
		"instance_id": result.InstanceID, "timezone": result.Timezone,
		"window_start": result.WindowStart, "window_end": result.WindowEnd,
		"days": query.Days, "limit": query.Limit,
		"policy": result.Policy, "backups": result.Backups,
		"backup_total":      result.BackupTotal,
		"collection_errors": result.CollectionErrors,
		"time_note":         "policy.start_time_utc 是华为云返回的 UTC 备份窗口；policy.window_local 与 backups 的 begin/end 已按 timezone 转换。未返回 policy 不得臆造备份窗口。",
	}
	s.appendToolResult(ctx, userID, session.SessionID, "", data, obs)
	return obs.Status == EvidenceStatusOK
}

func rdsBackupSummary(result *RDSBackupResult) string {
	if result == nil {
		return "RDS 备份证据为空"
	}
	if result.Status != EvidenceStatusOK {
		return fmt.Sprintf("RDS 备份证据 %s（%s）", result.Status, result.Reason)
	}
	policy := "未返回备份策略"
	if result.Policy != nil && result.Policy.StartTimeUTC != "" {
		policy = "备份窗口 " + result.Policy.StartTimeUTC + " UTC"
		if result.Policy.WindowLocal != "" {
			policy += " / " + result.Policy.WindowLocal + " " + result.Timezone
		}
	}
	return fmt.Sprintf("RDS 备份证据 ok（实例 %s，%s，最近备份 %d 条）",
		result.InstanceID, policy, len(result.Backups))
}
