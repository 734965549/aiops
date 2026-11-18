package identity

import (
	"context"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	identitydomain "github.com/734965549/aiops/internal/identity/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// stubGrantRepo 只实现 LoadUserGrantContext：统一授权服务判定数据范围时只调用该方法，
// 其余 AccessControlRepository 方法通过内嵌接口保留，误调用会 panic 以便暴露测试假设变化。
type stubGrantRepo struct {
	identitydomain.AccessControlRepository
	grant *identitydomain.UserGrantContext
}

func (s stubGrantRepo) LoadUserGrantContext(context.Context, string) (*identitydomain.UserGrantContext, error) {
	return s.grant, nil
}

// realGuard 用真实 AuthorizationService 构造告警数据范围守卫。
func realGuard(t *testing.T, scopes ...identitydomain.DataScope) *AlertScopeAuthorizer {
	t.Helper()
	svc := identityapp.NewAuthorizationService(stubGrantRepo{grant: &identitydomain.UserGrantContext{
		Roles:      []identitydomain.Role{{ID: "r1", Code: "operator"}},
		DataScopes: scopes,
	}})
	return NewAlertScopeAuthorizer(svc)
}

func teamScope(teams ...string) identitydomain.DataScope {
	values := make([]any, 0, len(teams))
	for _, team := range teams {
		values = append(values, team)
	}
	return identitydomain.DataScope{
		Code: "team-sre", ScopeType: identitydomain.DataScopeTeam,
		ScopeConfig: map[string]any{"team_ids": values},
	}
}

func TestAlertScopeWithRealAuthorizationService(t *testing.T) {
	cases := []struct {
		name    string
		scopes  []identitydomain.DataScope
		alert   *aiapp.AlertContext
		allowed bool
	}{
		{
			name:    "all 范围不看对象上下文",
			scopes:  []identitydomain.DataScope{{Code: "all-data", ScopeType: identitydomain.DataScopeAll}},
			alert:   &aiapp.AlertContext{ID: "a1"},
			allowed: true,
		},
		{
			name:    "无数据范围不过滤",
			scopes:  nil,
			alert:   &aiapp.AlertContext{ID: "a1"},
			allowed: true,
		},
		{
			name:    "team 范围命中标签",
			scopes:  []identitydomain.DataScope{teamScope("sre")},
			alert:   &aiapp.AlertContext{ID: "a1", Labels: map[string]string{"team": "sre"}},
			allowed: true,
		},
		{
			name:    "team 范围标签不匹配",
			scopes:  []identitydomain.DataScope{teamScope("sre")},
			alert:   &aiapp.AlertContext{ID: "a1", Labels: map[string]string{"team": "payments"}},
			allowed: false,
		},
		{
			name:    "team 范围缺失对象上下文时失败关闭",
			scopes:  []identitydomain.DataScope{teamScope("sre")},
			alert:   &aiapp.AlertContext{ID: "a1", Labels: map[string]string{"env": "prod"}},
			allowed: false,
		},
		{
			name: "custom 范围按归属负责人匹配",
			scopes: []identitydomain.DataScope{{
				Code: "owner-scope", ScopeType: identitydomain.DataScopeCustom,
				ScopeConfig: map[string]any{"owner_ids": []any{"user-9"}},
			}},
			alert:   &aiapp.AlertContext{ID: "a1", OwnerUserID: "user-9"},
			allowed: true,
		},
		{
			name: "custom 范围归属不匹配",
			scopes: []identitydomain.DataScope{{
				Code: "owner-scope", ScopeType: identitydomain.DataScopeCustom,
				ScopeConfig: map[string]any{"owner_ids": []any{"user-9"}},
			}},
			alert:   &aiapp.AlertContext{ID: "a1", OwnerUserID: "user-8"},
			allowed: false,
		},
		{
			name: "tag 范围按 environment 字段匹配",
			scopes: []identitydomain.DataScope{{
				Code: "prod-tag", ScopeType: identitydomain.DataScopeTag,
				ScopeConfig: map[string]any{"tags": []any{"prod"}},
			}},
			alert:   &aiapp.AlertContext{ID: "a1", Environment: "prod"},
			allowed: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guard := realGuard(t, tc.scopes...)
			err := guard.AuthorizeAlert(context.Background(), "user-1", tc.alert)
			if tc.allowed {
				if err != nil {
					t.Fatalf("expected allowed, got %v", err)
				}
				return
			}
			if apperr.CodeOf(err) != apperr.CodePermissionDenied {
				t.Fatalf("expected permission denied, got %v", err)
			}
		})
	}
}
