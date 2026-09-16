// Package authz 将 identity 授权服务适配为 asset application 的 AuthorizationPort。
package authz

import (
	"context"

	assetapp "github.com/734965549/aiops/internal/asset/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// Adapter 包装 identity AuthorizationService，暴露资产同步数据范围校验端口。
type Adapter struct {
	svc *identityapp.AuthorizationService
}

// NewAdapter 构造适配器。
func NewAdapter(svc *identityapp.AuthorizationService) *Adapter {
	return &Adapter{svc: svc}
}

// Authorize 委托 identity 授权服务执行 RBAC + 数据范围校验（SkipDataScope=false）。
// identity 的 deny 返回 (res, CodePermissionDenied err)，此处统一转为 (Allowed=false, err=nil)，
// 便于调用方按业务语义区分「数据范围拒绝」与「系统错误」。
func (a *Adapter) Authorize(ctx context.Context, in assetapp.AuthorizationInput) (*assetapp.AuthorizationResult, error) {
	if a == nil || a.svc == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "authorization service is not configured")
	}
	res, err := a.svc.Authorize(ctx, identityapp.AuthorizationInput{
		UserID:        in.UserID,
		Resource:      in.Resource,
		Action:        in.Action,
		ObjectOwner:   in.ObjectOwner,
		ObjectDept:    in.ObjectDept,
		ObjectTeam:    in.ObjectTeam,
		ObjectRegion:  in.ObjectRegion,
		ObjectTags:    in.ObjectTags,
		SkipDataScope: false,
	})
	if err != nil {
		if apperr.CodeOf(err) == apperr.CodePermissionDenied && res != nil {
			return &assetapp.AuthorizationResult{Allowed: false, Reason: res.Reason}, nil
		}
		return nil, err
	}
	if res == nil {
		return nil, apperr.New(apperr.CodeInternal, "authorization returned nil result")
	}
	return &assetapp.AuthorizationResult{Allowed: res.Allowed, Reason: res.Reason}, nil
}

// ResolveAccessibleOwnerTeams 委托 identity 授权服务解析用户可访问的 owner_team 集合。
func (a *Adapter) ResolveAccessibleOwnerTeams(ctx context.Context, userID string) ([]string, bool, error) {
	if a == nil || a.svc == nil {
		return nil, false, apperr.New(apperr.CodeUnavailable, "authorization service is not configured")
	}
	return a.svc.ResolveAccessibleOwnerTeams(ctx, userID)
}

var _ assetapp.AuthorizationPort = (*Adapter)(nil)
