package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// 会话只读分享（ops/ai-contract.md §10.18）：
// 仅平台内已登录且具备 app:ai:chat 的用户可访问；token 只存哈希，owner 可撤销。
const (
	chatShareDefaultTTL = 7 * 24 * time.Hour
	chatShareMinTTL     = time.Hour
	chatShareMaxTTL     = 30 * 24 * time.Hour
	// chatShareMaxActive 单会话同时有效的分享链接上限，避免无限生成。
	chatShareMaxActive = 10
)

// CreateChatShareInput 创建分享入参；ExpiresInHours 为 0 时使用默认有效期。
type CreateChatShareInput struct {
	UserID         string
	SessionID      string
	ExpiresInHours int
}

// ChatShareView 分享的只读视图；不含 token 与 token 哈希。
type ChatShareView struct {
	ShareID   string    `json:"share_id"`
	SessionID string    `json:"session_id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// CreatedChatShare 创建响应：token 与分享链接只在这里返回一次。
type CreatedChatShare struct {
	ChatShareView
	Token    string `json:"token"`
	ShareURL string `json:"share_url"`
}

// SetShares 注入分享仓储；未注入时分享能力不可用，其余对话能力不受影响。
func (s *ChatService) SetShares(repo domain.SessionShareRepository) {
	if s == nil {
		return
	}
	s.shares = repo
}

// CreateShare 为当前用户的会话生成只读分享链接。
func (s *ChatService) CreateShare(ctx context.Context, in CreateChatShareInput) (*CreatedChatShare, error) {
	if s == nil || s.sessions == nil || s.shares == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat sharing is not enabled")
	}
	session, err := s.loadOwnedSession(ctx, in.UserID, in.SessionID)
	if err != nil {
		return nil, err
	}
	ttl, err := chatShareTTL(in.ExpiresInHours)
	if err != nil {
		return nil, err
	}
	existing, err := s.shares.ListBySession(ctx, session.SessionID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai chat shares failed")
	}
	now := s.now()
	active := 0
	for _, share := range existing {
		if share.Status == domain.ShareStatusActive && now.Before(share.ExpiresAt) {
			active++
		}
	}
	if active >= chatShareMaxActive {
		return nil, apperr.New(apperr.CodeFailedPrecondition,
			"too many active share links for this session; revoke one before creating another")
	}
	token, err := domain.NewShareToken()
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "generate share token failed")
	}
	share := domain.SessionShare{
		ShareID:     uuid.NewString(),
		SessionID:   session.SessionID,
		OwnerUserID: session.UserID,
		TokenHash:   domain.ShareTokenHash(token),
		Status:      domain.ShareStatusActive,
		ExpiresAt:   now.Add(ttl),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.shares.Create(ctx, share); err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "create ai chat share failed")
	}
	s.recordAudit(ctx, session.UserID, "ai", session.SessionID, AuditChatShareCreate, map[string]any{
		"session_id": session.SessionID,
		"share_id":   share.ShareID,
		"expires_at": share.ExpiresAt,
		"result":     "success",
	})
	return &CreatedChatShare{
		ChatShareView: toChatShareView(share),
		Token:         token,
		ShareURL:      "/ai-assistant/shared/" + token,
	}, nil
}

// ListShares 返回会话的分享记录（含已撤销/已过期），仅会话所有者可查看。
func (s *ChatService) ListShares(ctx context.Context, userID, sessionID string) ([]ChatShareView, error) {
	if s == nil || s.sessions == nil || s.shares == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat sharing is not enabled")
	}
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.shares.ListBySession(ctx, session.SessionID)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list ai chat shares failed")
	}
	out := make([]ChatShareView, 0, len(rows))
	for _, row := range rows {
		out = append(out, toChatShareView(row))
	}
	return out, nil
}

// RevokeShare 撤销分享链接；已撤销时幂等返回当前视图。
func (s *ChatService) RevokeShare(ctx context.Context, userID, sessionID, shareID string) (*ChatShareView, error) {
	if s == nil || s.sessions == nil || s.shares == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat sharing is not enabled")
	}
	session, err := s.loadOwnedSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	share, err := s.shares.Get(ctx, strings.TrimSpace(shareID))
	if err != nil {
		if errors.Is(err, domain.ErrShareNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "share link not found")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai chat share failed")
	}
	// 分享不属于该会话时按不存在处理，避免用错误信息探测其它会话。
	if share.SessionID != session.SessionID {
		return nil, apperr.New(apperr.CodeNotFound, "share link not found")
	}
	now := s.now()
	if share.Status != domain.ShareStatusRevoked {
		if err := s.shares.Revoke(ctx, share.ShareID, now); err != nil {
			if errors.Is(err, domain.ErrShareNotFound) {
				return nil, apperr.New(apperr.CodeNotFound, "share link not found")
			}
			return nil, apperr.Wrap(err, apperr.CodeInternal, "revoke ai chat share failed")
		}
		share.Status = domain.ShareStatusRevoked
		share.UpdatedAt = now
	}
	s.recordAudit(ctx, session.UserID, "ai", session.SessionID, AuditChatShareRevoke, map[string]any{
		"session_id": session.SessionID,
		"share_id":   share.ShareID,
		"result":     "success",
	})
	view := toChatShareView(*share)
	return &view, nil
}

// ViewSharedSession 通过分享 token 读取只读会话视图；不校验会话归属。
// 过期、撤销、不存在统一返回 404，避免泄露链接是否存在过。
func (s *ChatService) ViewSharedSession(ctx context.Context, userID, token string) (*ChatSessionDetail, error) {
	if s == nil || s.sessions == nil || s.shares == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "ai chat sharing is not enabled")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, apperr.New(apperr.CodeUnauthenticated, "missing user identity")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "share token is required")
	}
	share, err := s.shares.GetByTokenHash(ctx, domain.ShareTokenHash(token))
	if err != nil {
		if errors.Is(err, domain.ErrShareNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "share link is not available")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load ai chat share failed")
	}
	if err := share.Usable(s.now()); err != nil {
		return nil, apperr.New(apperr.CodeNotFound, "share link is not available")
	}
	session, err := s.sessions.Get(ctx, share.SessionID)
	if err != nil {
		if errors.Is(err, domain.ErrSessionNotFound) {
			return nil, apperr.New(apperr.CodeNotFound, "share link is not available")
		}
		return nil, apperr.Wrap(err, apperr.CodeInternal, "load shared ai chat session failed")
	}
	rows, err := s.sessions.ListRecentMessages(ctx, session.SessionID, 200)
	if err != nil {
		return nil, apperr.Wrap(err, apperr.CodeInternal, "list shared ai chat messages failed")
	}
	messages := make([]ChatMessageView, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toMessageView(row))
	}
	s.recordAudit(ctx, userID, "ai", session.SessionID, AuditChatShareView, map[string]any{
		"session_id": session.SessionID,
		"share_id":   share.ShareID,
		"result":     "success",
	})
	return &ChatSessionDetail{Session: toSessionView(*session), Messages: messages}, nil
}

func chatShareTTL(hours int) (time.Duration, error) {
	if hours == 0 {
		return chatShareDefaultTTL, nil
	}
	if hours < 0 {
		return 0, apperr.New(apperr.CodeInvalidArgument, "expires_in_hours must be positive")
	}
	ttl := time.Duration(hours) * time.Hour
	if ttl < chatShareMinTTL || ttl > chatShareMaxTTL {
		return 0, apperr.Newf(apperr.CodeInvalidArgument,
			"expires_in_hours must be between %d and %d", int(chatShareMinTTL.Hours()), int(chatShareMaxTTL.Hours()))
	}
	return ttl, nil
}

func toChatShareView(share domain.SessionShare) ChatShareView {
	return ChatShareView{
		ShareID:   share.ShareID,
		SessionID: share.SessionID,
		Status:    share.Status,
		ExpiresAt: share.ExpiresAt,
		CreatedAt: share.CreatedAt,
	}
}
