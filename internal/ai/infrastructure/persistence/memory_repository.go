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
	"gorm.io/gorm/clause"
)

// memoryCaseModel 对应 ai_memory_case（migration 0051）。
type memoryCaseModel struct {
	database.BaseModel
	CaseID          string `gorm:"column:case_id;type:varchar(64);uniqueIndex;not null"`
	SourceType      string `gorm:"column:source_type;type:varchar(32);not null"`
	SourceMessageID string `gorm:"column:source_message_id;type:varchar(64);not null;default:''"`
	SessionID       string `gorm:"column:session_id;type:varchar(64);not null;default:''"`
	UserID          string `gorm:"column:user_id;type:varchar(64);not null"`
	ApplicationID   string `gorm:"column:application_id;type:varchar(64);not null;default:''"`
	ResourceID      string `gorm:"column:resource_id;type:varchar(128);not null;default:''"`
	ResourceType    string `gorm:"column:resource_type;type:varchar(64);not null;default:''"`
	AlertSignature  string `gorm:"column:alert_signature;type:varchar(255);not null;default:''"`
	Keywords        string `gorm:"column:keywords;type:text;not null;default:''"`
	Summary         string `gorm:"column:summary;type:text;not null;default:''"`
	Recommendations string `gorm:"column:recommendations;type:text;not null;default:''"`
	RiskLevel       string `gorm:"column:risk_level;type:varchar(16);not null;default:''"`
	QualityScore    int    `gorm:"column:quality_score;not null;default:0"`
	Status          string `gorm:"column:status;type:varchar(16);not null;default:'active'"`
}

func (memoryCaseModel) TableName() string { return "ai_memory_case" }

// MemoryCaseRepository 是 ai_memory_case 的 GORM 实现。
type MemoryCaseRepository struct {
	db *gorm.DB
}

func NewMemoryCaseRepository(db *gorm.DB) *MemoryCaseRepository {
	return &MemoryCaseRepository{db: db}
}

// Create 幂等写入记忆条目：同一用户同一来源消息只沉淀一条，重复写入返回 created=false。
func (r *MemoryCaseRepository) Create(ctx context.Context, c domain.MemoryCase) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("ai memory repository is not configured")
	}
	recommendations, err := marshalRecommendations(c.Recommendations)
	if err != nil {
		return false, err
	}
	row := memoryCaseModel{
		BaseModel:       database.BaseModel{CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt},
		CaseID:          strings.TrimSpace(c.CaseID),
		SourceType:      string(c.SourceType),
		SourceMessageID: strings.TrimSpace(c.SourceMessageID),
		SessionID:       strings.TrimSpace(c.SessionID),
		UserID:          strings.TrimSpace(c.UserID),
		ApplicationID:   strings.TrimSpace(c.ApplicationID),
		ResourceID:      strings.TrimSpace(c.ResourceID),
		ResourceType:    strings.TrimSpace(c.ResourceType),
		AlertSignature:  strings.TrimSpace(c.AlertSignature),
		Keywords:        c.Keywords,
		Summary:         c.Summary,
		Recommendations: recommendations,
		RiskLevel:       strings.TrimSpace(c.RiskLevel),
		QualityScore:    c.QualityScore,
		Status:          string(c.Status),
	}
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// FindBySourceMessage 按用户与来源消息查找案例；不存在返回 (nil, nil)。
func (r *MemoryCaseRepository) FindBySourceMessage(ctx context.Context, userID, sourceMessageID string) (*domain.MemoryCase, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai memory repository is not configured")
	}
	userID = strings.TrimSpace(userID)
	sourceMessageID = strings.TrimSpace(sourceMessageID)
	if userID == "" || sourceMessageID == "" {
		return nil, nil
	}
	var row memoryCaseModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND source_message_id = ?", userID, sourceMessageID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	c, err := toMemoryCaseDomain(&row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateQuality 只更新质量分、状态与更新时间，不改写案例内容。
func (r *MemoryCaseRepository) UpdateQuality(ctx context.Context, c domain.MemoryCase) error {
	if r == nil || r.db == nil {
		return errors.New("ai memory repository is not configured")
	}
	res := r.db.WithContext(ctx).Model(&memoryCaseModel{}).
		Where("case_id = ? AND user_id = ?", strings.TrimSpace(c.CaseID), strings.TrimSpace(c.UserID)).
		Updates(map[string]any{
			"quality_score": c.QualityScore,
			"status":        string(c.Status),
			"updated_at":    c.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("ai memory case not found")
	}
	return nil
}

// Search 按告警指纹 / 应用 / 资源 / 关键词检索当前用户的有效案例。
//
// 优先级与 ai_memory_case 的 match_rank 一致：指纹命中 3、应用或资源命中 2、关键词命中 1。
// 只返回 status=active 且 user_id 匹配的记录，保证跨用户隔离。
func (r *MemoryCaseRepository) Search(ctx context.Context, filter domain.MemoryCaseFilter) ([]domain.MemoryCase, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai memory repository is not configured")
	}
	userID := strings.TrimSpace(filter.UserID)
	if userID == "" {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 || limit > domain.MemoryCaseMaxMatches {
		limit = domain.MemoryCaseMaxMatches
	}
	signature := strings.TrimSpace(filter.AlertSignature)
	applicationID := strings.TrimSpace(filter.ApplicationID)
	resourceID := strings.TrimSpace(filter.ResourceID)
	keywords := strings.TrimSpace(filter.Keywords)

	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if signature != "" {
		conditions = append(conditions, "alert_signature = ?")
		args = append(args, signature)
	}
	if applicationID != "" {
		conditions = append(conditions, "application_id = ?")
		args = append(args, applicationID)
	}
	if resourceID != "" {
		conditions = append(conditions, "resource_id = ?")
		args = append(args, resourceID)
	}
	if keywords != "" {
		conditions = append(conditions, "to_tsvector('simple', keywords) @@ plainto_tsquery('simple', ?)")
		args = append(args, keywords)
	}
	if len(conditions) == 0 {
		return nil, nil
	}

	// match_rank 与 domain.MemoryCaseFilter.MatchPriority 保持同一口径。
	const rankExpr = "CASE " +
		"WHEN (?) <> '' AND alert_signature = (?) THEN 3 " +
		"WHEN ((?) <> '' AND application_id = (?)) OR ((?) <> '' AND resource_id = (?)) THEN 2 " +
		"WHEN (?) <> '' THEN 1 ELSE 0 END AS match_rank"
	query := r.db.WithContext(ctx).
		Select("*, "+rankExpr,
			signature, signature,
			applicationID, applicationID, resourceID, resourceID,
			keywords).
		Where("user_id = ? AND status = ?", userID, string(domain.MemoryCaseStatusActive)).
		Where("("+strings.Join(conditions, " OR ")+")", args...).
		Order("match_rank DESC, quality_score DESC, updated_at DESC").
		Limit(limit)

	var rows []memoryCaseModel
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.MemoryCase, 0, len(rows))
	for i := range rows {
		c, err := toMemoryCaseDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ListForAggregation 返回指定用户在某应用下的高质量有效案例（供知识聚合）。
func (r *MemoryCaseRepository) ListForAggregation(ctx context.Context, userID, applicationID string, limit int) ([]domain.MemoryCase, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai memory repository is not configured")
	}
	userID = strings.TrimSpace(userID)
	applicationID = strings.TrimSpace(applicationID)
	if userID == "" || applicationID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows []memoryCaseModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND application_id = ? AND status = ?", userID, applicationID, string(domain.MemoryCaseStatusActive)).
		Order("quality_score DESC, updated_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.MemoryCase, 0, len(rows))
	for i := range rows {
		c, err := toMemoryCaseDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// List 是管理端分页列表：支持归属用户、状态、应用/资源与关键词过滤。
func (r *MemoryCaseRepository) List(ctx context.Context, filter domain.MemoryCaseListFilter) ([]domain.MemoryCase, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai memory repository is not configured")
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query := r.db.WithContext(ctx).Model(&memoryCaseModel{})
	if userID := strings.TrimSpace(filter.UserID); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if applicationID := strings.TrimSpace(filter.ApplicationID); applicationID != "" {
		query = query.Where("application_id = ?", applicationID)
	}
	if resourceID := strings.TrimSpace(filter.ResourceID); resourceID != "" {
		query = query.Where("resource_id = ?", resourceID)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"(summary ILIKE ? OR keywords ILIKE ? OR application_id ILIKE ? OR resource_id ILIKE ?)",
			like, like, like, like,
		)
	}
	var rows []memoryCaseModel
	if err := query.Order("updated_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.MemoryCase, 0, len(rows))
	for i := range rows {
		c, err := toMemoryCaseDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Get 按业务 ID 读取案例；不存在返回 domain.ErrMemoryCaseNotFound。
func (r *MemoryCaseRepository) Get(ctx context.Context, caseID string) (*domain.MemoryCase, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("ai memory repository is not configured")
	}
	caseID = strings.TrimSpace(caseID)
	if caseID == "" {
		return nil, domain.ErrMemoryCaseNotFound
	}
	var row memoryCaseModel
	if err := r.db.WithContext(ctx).Where("case_id = ?", caseID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMemoryCaseNotFound
		}
		return nil, err
	}
	c, err := toMemoryCaseDomain(&row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateStatus 只更新状态与时间；case_id 是业务唯一键。
func (r *MemoryCaseRepository) UpdateStatus(ctx context.Context, caseID string, status domain.MemoryCaseStatus, updatedAt time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("ai memory repository is not configured")
	}
	res := r.db.WithContext(ctx).Model(&memoryCaseModel{}).
		Where("case_id = ?", strings.TrimSpace(caseID)).
		Updates(map[string]any{
			"status":     string(status),
			"updated_at": updatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrMemoryCaseNotFound
	}
	return nil
}

func toMemoryCaseDomain(row *memoryCaseModel) (domain.MemoryCase, error) {
	source := domain.MemoryCaseSource(row.SourceType)
	if !source.IsValid() {
		source = domain.MemoryCaseSourceChatMessage
	}
	status := domain.MemoryCaseStatus(row.Status)
	if !status.IsValid() {
		status = domain.MemoryCaseStatusActive
	}
	return domain.MemoryCase{
		CaseID:          row.CaseID,
		SourceType:      source,
		SourceMessageID: row.SourceMessageID,
		SessionID:       row.SessionID,
		UserID:          row.UserID,
		ApplicationID:   row.ApplicationID,
		ResourceID:      row.ResourceID,
		ResourceType:    row.ResourceType,
		AlertSignature:  row.AlertSignature,
		Keywords:        row.Keywords,
		Summary:         row.Summary,
		Recommendations: unmarshalRecommendations(row.Recommendations),
		RiskLevel:       row.RiskLevel,
		QualityScore:    row.QualityScore,
		Status:          status,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

func marshalRecommendations(items []string) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalRecommendations(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}
