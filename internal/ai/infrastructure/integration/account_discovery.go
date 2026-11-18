// Package integration 将 Integration 账号能力适配为 AI 账号发现端口。
package integration

import (
	"context"
	"strings"
	"time"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	integapp "github.com/734965549/aiops/internal/integration/application"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const (
	defaultAccountListTimeout   = 5 * time.Second
	accountDiscoveryMaxAccounts = 10
)

// AccountLister 是账号发现依赖的最小只读能力（由 integapp.AccountService 实现）。
type AccountLister interface {
	List(ctx context.Context, q integapp.ListAccountsQuery) ([]integapp.AccountDTO, int64, error)
}

// AccountDiscovery 实现 aiapp.AccountListPort。
// 只保留当前用户数据范围内的启用账号，与 metrics.query 取数口径一致。
type AccountDiscovery struct {
	accounts AccountLister
	access   aiapp.AccountAccessAuthorizer
	timeout  time.Duration
}

func NewAccountDiscovery(
	accounts AccountLister,
	access aiapp.AccountAccessAuthorizer,
	timeout time.Duration,
) *AccountDiscovery {
	if timeout <= 0 {
		timeout = defaultAccountListTimeout
	}
	return &AccountDiscovery{accounts: accounts, access: access, timeout: timeout}
}

var _ aiapp.AccountListPort = (*AccountDiscovery)(nil)

// List 返回当前用户可见的启用账号摘要（上限 10 个，Total 保留真实总数）。
func (a *AccountDiscovery) List(ctx context.Context, userID string) (*aiapp.AccountListResult, error) {
	if a == nil || a.accounts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai account discovery is not configured")
	}
	queryCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	enabled := true
	rows, total, err := a.accounts.List(queryCtx, integapp.ListAccountsQuery{
		Page: 1, PageSize: 100, Enabled: &enabled,
	})
	if err != nil {
		reason := "list integration accounts failed"
		if ae := apperr.FromError(err); ae.Code != apperr.CodeInternal && strings.TrimSpace(ae.Message) != "" {
			reason = ae.Message
		}
		return &aiapp.AccountListResult{Status: aiapp.EvidenceStatusError, Reason: reason}, nil
	}
	result := &aiapp.AccountListResult{Status: aiapp.EvidenceStatusOK, Total: int(total)}
	for _, row := range rows {
		if len(result.Accounts) >= accountDiscoveryMaxAccounts {
			break
		}
		if a.access != nil && strings.TrimSpace(userID) != "" {
			ref := aiapp.AccountRef{
				AccountID: row.AccountID,
				OwnerTeam: row.OwnerTeam,
				Regions:   append([]string(nil), row.Regions...),
			}
			if err := a.access.Authorize(queryCtx, userID, ref); err != nil {
				if apperr.CodeOf(err) == apperr.CodePermissionDenied {
					continue
				}
				return &aiapp.AccountListResult{
					Status: aiapp.EvidenceStatusError,
					Reason: "authorize integration account failed",
				}, nil
			}
		}
		result.Accounts = append(result.Accounts, aiapp.AccountSummary{
			AccountID: row.AccountID,
			Name:      row.Name,
			Provider:  row.Provider,
			Regions:   append([]string(nil), row.Regions...),
		})
	}
	return result, nil
}
