// Package persistence 实现 AI 运维助手会话的 PostgreSQL 仓储。
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type sessionModel struct {
	database.BaseModel
	SessionID   string `gorm:"column:session_id;type:varchar(64);uniqueIndex;not null"`
	UserID      string `gorm:"column:user_id;type:varchar(64);not null"`
	Title       string `gorm:"column:title;type:varchar(255);not null;default:''"`
	ContextJSON []byte `gorm:"column:context_json;type:jsonb;not null"`
	Status      string `gorm:"column:status;type:varchar(16);not null;default:'active'"`
}

func (sessionModel) TableName() string { return "ai_session" }

type sessionMessageModel struct {
	database.BaseModel
	MessageID      string `gorm:"column:message_id;type:varchar(64);uniqueIndex;not null"`
	SessionID      string `gorm:"column:session_id;type:varchar(64);not null"`
	Role           string `gorm:"column:role;type:varchar(16);not null"`
	Content        string `gorm:"column:content;type:text;not null;default:''"`
	ToolCallsJSON  []byte `gorm:"column:tool_calls_json;type:jsonb;not null"`
	ReferencesJSON []byte `gorm:"column:references_json;type:jsonb;not null"`
	Feedback       string `gorm:"column:feedback;type:varchar(16);not null;default:''"`
}

func (sessionMessageModel) TableName() string { return "ai_session_message" }

// SessionRepository 是 ai_session / ai_session_message 的 GORM 实现。
type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, session domain.Session) error {
	if r == nil || r.db == nil {
		return errors.New("ai session repository is not configured")
	}
	contextJSON, err := marshalContext(session.Context)
	if err != nil {
		return err
	}
	row := sessionModel{
		BaseModel:   database.BaseModel{CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt},
		SessionID:   strings.TrimSpace(session.SessionID),
		UserID:      strings.TrimSpace(session.UserID),
		Title:       session.Title,
		ContextJSON: contextJSON,
		Status:      string(session.Status),
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

func (r *SessionRepository) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session repository is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, domain.ErrSessionNotFound
	}
	var row sessionModel
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	session, err := toSessionDomain(&row)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionRepository) ListByUser(ctx context.Context, userID string, limit int) ([]domain.Session, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session repository is not configured")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []sessionModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Session, 0, len(rows))
	for i := range rows {
		session, err := toSessionDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, nil
}

// AppendMessage 在同一事务内写入消息并推进会话（标题、updated_at）。
func (r *SessionRepository) AppendMessage(ctx context.Context, message domain.Message, touch domain.SessionTouch) error {
	if r == nil || r.db == nil {
		return errors.New("ai session repository is not configured")
	}
	sessionID := strings.TrimSpace(message.SessionID)
	messageID := strings.TrimSpace(message.MessageID)
	if sessionID == "" || messageID == "" {
		return errors.New("ai session message requires session_id and message_id")
	}
	toolCallsJSON, err := marshalToolCalls(message.ToolCalls)
	if err != nil {
		return err
	}
	referencesJSON, err := marshalReferences(message.References)
	if err != nil {
		return err
	}
	row := sessionMessageModel{
		BaseModel:      database.BaseModel{CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt},
		MessageID:      messageID,
		SessionID:      sessionID,
		Role:           string(message.Role),
		Content:        message.Content,
		ToolCallsJSON:  toolCallsJSON,
		ReferencesJSON: referencesJSON,
		Feedback:       string(message.Feedback),
	}
	updatedAt := touch.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = message.UpdatedAt
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": updatedAt}
		if title := strings.TrimSpace(touch.Title); title != "" {
			updates["title"] = title
		}
		if touch.Context != nil {
			contextJSON, err := marshalContext(*touch.Context)
			if err != nil {
				return err
			}
			updates["context_json"] = contextJSON
		}
		res := tx.Model(&sessionModel{}).Where("session_id = ?", sessionID).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrSessionNotFound
		}
		return nil
	})
}

func (r *SessionRepository) ListMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session repository is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []sessionMessageModel
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for i := range rows {
		msg, err := toMessageDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

// ListRecentMessages 按时间倒序取最近 limit 条消息后再反转为升序，
// 避免长会话因先取最早消息而丢失最新对话。
func (r *SessionRepository) ListRecentMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session repository is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []sessionMessageModel
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		msg, err := toMessageDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

func (r *SessionRepository) GetMessage(ctx context.Context, sessionID, messageID string) (*domain.Message, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai session repository is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	if sessionID == "" || messageID == "" {
		return nil, domain.ErrMessageNotFound
	}
	var row sessionMessageModel
	if err := r.db.WithContext(ctx).
		Where("session_id = ? AND message_id = ?", sessionID, messageID).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMessageNotFound
		}
		return nil, err
	}
	msg, err := toMessageDomain(&row)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *SessionRepository) SetFeedback(ctx context.Context, sessionID, messageID string, feedback domain.Feedback, updatedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("ai session repository is not configured")
	}
	res := r.db.WithContext(ctx).Model(&sessionMessageModel{}).
		Where("session_id = ? AND message_id = ?", strings.TrimSpace(sessionID), strings.TrimSpace(messageID)).
		Updates(map[string]any{"feedback": string(feedback), "updated_at": updatedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrMessageNotFound
	}
	return nil
}

func toSessionDomain(row *sessionModel) (domain.Session, error) {
	context, err := unmarshalContext(row.ContextJSON)
	if err != nil {
		return domain.Session{}, err
	}
	status := domain.SessionStatus(row.Status)
	if !status.IsValid() {
		status = domain.SessionStatusActive
	}
	return domain.Session{
		SessionID: row.SessionID,
		UserID:    row.UserID,
		Title:     row.Title,
		Context:   context,
		Status:    status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

func toMessageDomain(row *sessionMessageModel) (domain.Message, error) {
	toolCalls, err := unmarshalToolCalls(row.ToolCallsJSON)
	if err != nil {
		return domain.Message{}, err
	}
	references, err := unmarshalReferences(row.ReferencesJSON)
	if err != nil {
		return domain.Message{}, err
	}
	return domain.Message{
		MessageID:  row.MessageID,
		SessionID:  row.SessionID,
		Role:       domain.MessageRole(row.Role),
		Content:    row.Content,
		ToolCalls:  toolCalls,
		References: references,
		Feedback:   domain.Feedback(row.Feedback),
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}, nil
}

func marshalContext(context domain.SessionContext) ([]byte, error) {
	return json.Marshal(context)
}

func unmarshalContext(raw []byte) (domain.SessionContext, error) {
	out := domain.SessionContext{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return domain.SessionContext{}, err
	}
	return out, nil
}

func marshalToolCalls(calls []domain.ToolCall) ([]byte, error) {
	if calls == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(calls)
}

func unmarshalToolCalls(raw []byte) ([]domain.ToolCall, error) {
	if len(raw) == 0 {
		return []domain.ToolCall{}, nil
	}
	var out []domain.ToolCall
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.ToolCall{}
	}
	return out, nil
}

func marshalReferences(refs []domain.Reference) ([]byte, error) {
	if refs == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(refs)
}

func unmarshalReferences(raw []byte) ([]domain.Reference, error) {
	if len(raw) == 0 {
		return []domain.Reference{}, nil
	}
	var out []domain.Reference
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []domain.Reference{}
	}
	return out, nil
}
