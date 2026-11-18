package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// fakeShareRepo 是内存分享仓储，用于 application 层测试。
type fakeShareRepo struct {
	shares map[string]domain.SessionShare
}

func newFakeShareRepo() *fakeShareRepo {
	return &fakeShareRepo{shares: map[string]domain.SessionShare{}}
}

func (f *fakeShareRepo) Create(_ context.Context, share domain.SessionShare) error {
	for _, existing := range f.shares {
		if existing.TokenHash == share.TokenHash {
			return errors.New("duplicate token hash")
		}
	}
	f.shares[share.ShareID] = share
	return nil
}

func (f *fakeShareRepo) Get(_ context.Context, shareID string) (*domain.SessionShare, error) {
	share, ok := f.shares[shareID]
	if !ok {
		return nil, domain.ErrShareNotFound
	}
	return &share, nil
}

func (f *fakeShareRepo) GetByTokenHash(_ context.Context, tokenHash string) (*domain.SessionShare, error) {
	for _, share := range f.shares {
		if share.TokenHash == tokenHash {
			copied := share
			return &copied, nil
		}
	}
	return nil, domain.ErrShareNotFound
}

func (f *fakeShareRepo) ListBySession(_ context.Context, sessionID string) ([]domain.SessionShare, error) {
	out := make([]domain.SessionShare, 0)
	for _, share := range f.shares {
		if share.SessionID == sessionID {
			out = append(out, share)
		}
	}
	return out, nil
}

func (f *fakeShareRepo) Revoke(_ context.Context, shareID string, updatedAt time.Time) error {
	share, ok := f.shares[shareID]
	if !ok {
		return domain.ErrShareNotFound
	}
	share.Status = domain.ShareStatusRevoked
	share.UpdatedAt = updatedAt
	f.shares[shareID] = share
	return nil
}

func TestCreateShareStoresHashAndDefaultTTL(t *testing.T) {
	svc, repo, _, _, audit, _ := newChatTestService(t)
	shares := newFakeShareRepo()
	svc.SetShares(shares)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	created, err := svc.CreateShare(context.Background(), CreateChatShareInput{
		UserID: "u-1", SessionID: "s-1",
	})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}
	if created.Token == "" || created.ShareURL != "/ai-assistant/shared/"+created.Token {
		t.Fatalf("unexpected share response: %+v", created)
	}
	stored, ok := shares.shares[created.ShareID]
	if !ok {
		t.Fatal("share was not persisted")
	}
	// 只存哈希：明文 token 不应出现在仓储里。
	if stored.TokenHash != domain.ShareTokenHash(created.Token) || stored.TokenHash == created.Token {
		t.Fatalf("unexpected token hash: %q", stored.TokenHash)
	}
	if !stored.ExpiresAt.Equal(svc.now().Add(chatShareDefaultTTL)) {
		t.Fatalf("expires_at = %v", stored.ExpiresAt)
	}
	if stored.Status != domain.ShareStatusActive || stored.OwnerUserID != "u-1" {
		t.Fatalf("unexpected stored share: %+v", stored)
	}
	// 审计只记录 share_id，不记录 token。
	for _, row := range audit.rows {
		if row.Action == AuditChatShareCreate {
			if _, leaked := row.Payload["token"]; leaked {
				t.Fatal("audit payload must not contain token")
			}
			if row.Payload["share_id"] != created.ShareID {
				t.Fatalf("unexpected audit payload: %+v", row.Payload)
			}
		}
	}
}

func TestCreateShareValidatesTTLAndOwner(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	svc.SetShares(newFakeShareRepo())
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	if _, err := svc.CreateShare(context.Background(), CreateChatShareInput{
		UserID: "u-1", SessionID: "s-1", ExpiresInHours: chatShareMaxHours() + 1,
	}); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument for too long ttl, got %v", err)
	}
	if _, err := svc.CreateShare(context.Background(), CreateChatShareInput{
		UserID: "u-1", SessionID: "s-1", ExpiresInHours: -1,
	}); err == nil || apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument for negative ttl, got %v", err)
	}
	if _, err := svc.CreateShare(context.Background(), CreateChatShareInput{
		UserID: "u-2", SessionID: "s-1",
	}); err == nil || apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied for non-owner, got %v", err)
	}
}

func TestCreateShareCapsActiveLinks(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	shares := newFakeShareRepo()
	svc.SetShares(shares)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	now := svc.now()
	for i := 0; i < chatShareMaxActive; i++ {
		shares.shares[string(rune('a'+i))] = domain.SessionShare{
			ShareID: string(rune('a' + i)), SessionID: "s-1", OwnerUserID: "u-1",
			TokenHash: "hash-" + string(rune('a'+i)), Status: domain.ShareStatusActive,
			ExpiresAt: now.Add(time.Hour),
		}
	}
	_, err := svc.CreateShare(context.Background(), CreateChatShareInput{UserID: "u-1", SessionID: "s-1"})
	if err == nil || apperr.CodeOf(err) != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func TestViewSharedSessionOnlyReadsActiveLinks(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	shares := newFakeShareRepo()
	svc.SetShares(shares)
	seedChatSession(t, repo, "u-1", domain.SessionContext{Page: "alerts"})
	repo.messages["s-1"] = append(repo.messages["s-1"],
		domain.Message{MessageID: "m-u", SessionID: "s-1", Role: domain.RoleUser, Content: "分析告警"},
		domain.Message{MessageID: "m-a", SessionID: "s-1", Role: domain.RoleAssistant, Content: chatAnswerJSON("CPU 持续升高")},
	)

	created, err := svc.CreateShare(context.Background(), CreateChatShareInput{UserID: "u-1", SessionID: "s-1"})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}
	// 另一名登录用户可通过分享读取。
	detail, err := svc.ViewSharedSession(context.Background(), "u-2", created.Token)
	if err != nil {
		t.Fatalf("view shared session: %v", err)
	}
	if detail.Session.SessionID != "s-1" || len(detail.Messages) != 2 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if detail.Messages[1].Answer == nil || detail.Messages[1].Answer.Summary != "CPU 持续升高" {
		t.Fatalf("assistant answer not decoded: %+v", detail.Messages[1])
	}

	// 撤销后不可访问。
	if _, err := svc.RevokeShare(context.Background(), "u-1", "s-1", created.ShareID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.ViewSharedSession(context.Background(), "u-2", created.Token); err == nil ||
		apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found after revoke, got %v", err)
	}

	// 未知 token 与空 token 的边界。
	if _, err := svc.ViewSharedSession(context.Background(), "u-2", "unknown"); err == nil ||
		apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found for unknown token, got %v", err)
	}
	if _, err := svc.ViewSharedSession(context.Background(), "u-2", "  "); err == nil ||
		apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument for empty token, got %v", err)
	}
}

func TestViewSharedSessionHidesExpiredAndDeletedSession(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	shares := newFakeShareRepo()
	svc.SetShares(shares)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	created, err := svc.CreateShare(context.Background(), CreateChatShareInput{UserID: "u-1", SessionID: "s-1"})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}
	// 手动过期。
	expired := shares.shares[created.ShareID]
	expired.ExpiresAt = svc.now().Add(-time.Minute)
	shares.shares[created.ShareID] = expired
	if _, err := svc.ViewSharedSession(context.Background(), "u-2", created.Token); err == nil ||
		apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found for expired share, got %v", err)
	}

	// 会话被删除后同样不可访问。
	enabled := shares.shares[created.ShareID]
	enabled.ExpiresAt = svc.now().Add(time.Hour)
	shares.shares[created.ShareID] = enabled
	delete(repo.sessions, "s-1")
	if _, err := svc.ViewSharedSession(context.Background(), "u-2", created.Token); err == nil ||
		apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found after session deleted, got %v", err)
	}
}

func TestRevokeShareRequiresOwnerAndMatchingSession(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	shares := newFakeShareRepo()
	svc.SetShares(shares)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	seedChatSession(t, repo, "u-1", domain.SessionContext{})

	created, err := svc.CreateShare(context.Background(), CreateChatShareInput{UserID: "u-1", SessionID: "s-1"})
	if err != nil {
		t.Fatalf("create share: %v", err)
	}
	if _, err := svc.RevokeShare(context.Background(), "u-2", "s-1", created.ShareID); err == nil ||
		apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("expected permission denied for non-owner, got %v", err)
	}
	// 分享不属于该会话时按不存在处理。
	if _, err := svc.RevokeShare(context.Background(), "u-1", "s-2", created.ShareID); err == nil ||
		apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("expected not found for session mismatch, got %v", err)
	}
	first, err := svc.RevokeShare(context.Background(), "u-1", "s-1", created.ShareID)
	if err != nil || first.Status != domain.ShareStatusRevoked {
		t.Fatalf("revoke: %+v err=%v", first, err)
	}
	// 重复撤销幂等。
	second, err := svc.RevokeShare(context.Background(), "u-1", "s-1", created.ShareID)
	if err != nil || second.Status != domain.ShareStatusRevoked {
		t.Fatalf("revoke twice: %+v err=%v", second, err)
	}
}

func TestShareFeatureUnavailableWithoutRepository(t *testing.T) {
	svc, repo, _, _, _, _ := newChatTestService(t)
	seedChatSession(t, repo, "u-1", domain.SessionContext{})
	if _, err := svc.CreateShare(context.Background(), CreateChatShareInput{UserID: "u-1", SessionID: "s-1"}); err == nil ||
		apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func chatShareMaxHours() int { return int(chatShareMaxTTL.Hours()) }
