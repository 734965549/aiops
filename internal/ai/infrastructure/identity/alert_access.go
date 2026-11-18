package identity

import (
	"context"
	"strings"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/734965549/aiops/pkg/logger"
)

// AlertScopeAuthorizer 使用统一授权服务校验用户对告警对象的数据范围。
//
// 只做数据范围判断：RBAC、工具权限已由路由中间件与工具网关各自完成，
// 这里不再重复拼接 resource/action，避免无对象上下文的对话被误判为缺少权限。
type AlertScopeAuthorizer struct {
	checker AuthorizationChecker
}

func NewAlertScopeAuthorizer(checker AuthorizationChecker) *AlertScopeAuthorizer {
	return &AlertScopeAuthorizer{checker: checker}
}

var _ aiapp.AlertScopeAuthorizer = (*AlertScopeAuthorizer)(nil)

func (a *AlertScopeAuthorizer) AuthorizeAlert(ctx context.Context, userID string, alert *aiapp.AlertContext) error {
	if a == nil || a.checker == nil {
		return apperr.New(apperr.CodeUnavailable, "alert data scope authorizer is not configured")
	}
	obj := aiapp.AlertObjectContextOf(alert)
	in := identityapp.AuthorizationInput{
		UserID:       strings.TrimSpace(userID),
		ObjectOwner:  obj.Owner,
		ObjectDept:   obj.Dept,
		ObjectTeam:   obj.Team,
		ObjectRegion: obj.Region,
		ObjectTags:   obj.Tags,
	}
	res, err := a.checker.Authorize(ctx, in)
	if err != nil {
		if ae := apperr.FromError(err); ae != nil && ae.Code == apperr.CodePermissionDenied {
			logAlertScopeDenied(ctx, alert, in.UserID, ae.Message)
		}
		return err
	}
	if res == nil || !res.Allowed {
		reason := "data scope denied"
		if res != nil && strings.TrimSpace(res.Reason) != "" {
			reason = res.Reason
		}
		logAlertScopeDenied(ctx, alert, in.UserID, reason)
		return apperr.New(apperr.CodePermissionDenied, reason)
	}
	return nil
}

// logAlertScopeDenied 记录拒绝事实（只含业务 ID 与原因，不含告警正文）。
func logAlertScopeDenied(ctx context.Context, alert *aiapp.AlertContext, userID, reason string) {
	alertID := ""
	if alert != nil {
		alertID = alert.ID
	}
	logger.From(ctx).Warn("ai alert data scope denied",
		logger.String("alert_id", alertID),
		logger.String("user_id", userID),
		logger.String("reason", reason),
	)
}
