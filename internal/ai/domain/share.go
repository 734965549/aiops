package domain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// 会话分享状态（ops/ai-contract.md §10.18）。
const (
	ShareStatusActive  = "active"
	ShareStatusRevoked = "revoked"
)

// 分享相关的领域错误。
var (
	ErrShareNotFound = errors.New("ai session share not found")
	ErrShareExpired  = errors.New("ai session share expired")
	ErrShareRevoked  = errors.New("ai session share revoked")
)

// SessionShare 是一次只读分享凭证。
// 只保存 token 的 SHA-256；明文 token 仅在创建响应里返回一次，之后无法再取回。
type SessionShare struct {
	ShareID     string
	SessionID   string
	OwnerUserID string
	TokenHash   string
	Status      string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewShareToken 生成 32 字节随机 token（base64url，无填充，43 字符）。
func NewShareToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ShareTokenHash 返回 token 的 SHA-256 十六进制摘要，用于落库与查询。
func ShareTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// Usable 判断分享是否仍可访问：已撤销或已过期都不可用。
func (s SessionShare) Usable(now time.Time) error {
	if s.Status == ShareStatusRevoked {
		return ErrShareRevoked
	}
	if s.Status != ShareStatusActive {
		return ErrShareRevoked
	}
	if !now.Before(s.ExpiresAt) {
		return ErrShareExpired
	}
	return nil
}

// SessionShareRepository 是分享凭证的持久化端口。
type SessionShareRepository interface {
	Create(ctx context.Context, share SessionShare) error
	Get(ctx context.Context, shareID string) (*SessionShare, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*SessionShare, error)
	ListBySession(ctx context.Context, sessionID string) ([]SessionShare, error)
	Revoke(ctx context.Context, shareID string, updatedAt time.Time) error
}
