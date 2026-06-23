package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/execution/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/google/uuid"
)

// MediumService 执行介体管理。
type MediumService struct {
	media domain.MediumRepository
	audit AuditRecorder
	now   func() time.Time
}

func NewMediumService(media domain.MediumRepository, audit AuditRecorder) *MediumService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &MediumService{media: media, audit: audit, now: time.Now}
}

type CreateMediumInput struct {
	MediumID          string
	Name              string
	MediumType        string
	Environment       string
	Region            string
	NetworkZone       string
	Capabilities      []string
	AllowedCommandIDs []string
	MaxRiskLevel      string
	Enabled           *bool
	Description       string
}

type MediumDTO struct {
	MediumID          string   `json:"medium_id"`
	Name              string   `json:"name"`
	MediumType        string   `json:"medium_type"`
	Environment       string   `json:"environment"`
	Region            string   `json:"region"`
	NetworkZone       string   `json:"network_zone"`
	Capabilities      []string `json:"capabilities"`
	AllowedCommandIDs []string `json:"allowed_command_ids,omitempty"`
	MaxRiskLevel      string   `json:"max_risk_level"`
	Enabled           bool     `json:"enabled"`
	HealthStatus      string   `json:"health_status"`
	Description       string   `json:"description,omitempty"`
	CreatedAt         int64    `json:"created_at"`
	UpdatedAt         int64    `json:"updated_at"`
}

type ListMediaQuery struct {
	Page        int
	PageSize    int
	Enabled     *bool
	Environment string
	MediumType  string
	Keyword     string
}

func (s *MediumService) Create(ctx context.Context, actor Actor, in CreateMediumInput) (*MediumDTO, error) {
	if s == nil || s.media == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "medium service is not enabled")
	}
	mediumType := domain.MediumType(strings.ToLower(strings.TrimSpace(in.MediumType)))
	if !mediumType.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid medium_type")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "name is required")
	}
	mediumID := strings.TrimSpace(in.MediumID)
	if mediumID == "" {
		mediumID = "med-" + uuid.NewString()
	}
	maxRisk := domain.RiskLevel(strings.ToLower(strings.TrimSpace(in.MaxRiskLevel)))
	if maxRisk == "" {
		maxRisk = domain.RiskHigh
	}
	if !maxRisk.IsValid() {
		return nil, apperr.New(apperr.CodeInvalidArgument, "invalid max_risk_level")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	now := s.now()
	m := &domain.ExecutionMedium{
		MediumID: mediumID, Name: name, MediumType: mediumType,
		Environment: strings.TrimSpace(in.Environment), Region: strings.TrimSpace(in.Region),
		NetworkZone: strings.TrimSpace(in.NetworkZone),
		Capabilities: cloneStringSlice(in.Capabilities), AllowedCommandIDs: cloneStringSlice(in.AllowedCommandIDs),
		MaxRiskLevel: maxRisk, Enabled: enabled, HealthStatus: domain.MediumHealthUnknown,
		Description: strings.TrimSpace(in.Description), CreatedAt: now, UpdatedAt: now,
	}
	if err := s.media.Create(ctx, m); err != nil {
		return nil, wrapExecError(err, "create execution medium failed")
	}
	s.recordAudit(ctx, actor.UserID, AuditMediumCreate, m.MediumID, map[string]any{
		"medium_id": m.MediumID, "medium_type": string(m.MediumType), "environment": m.Environment,
	})
	return toMediumDTO(*m), nil
}

func (s *MediumService) List(ctx context.Context, q ListMediaQuery) ([]MediumDTO, int64, error) {
	if s == nil || s.media == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "medium service is not enabled")
	}
	page, pageSize := normalizePage(q.Page, q.PageSize)
	filter := domain.MediumFilter{
		Enabled: q.Enabled, Environment: strings.TrimSpace(q.Environment),
		MediumType: strings.TrimSpace(q.MediumType), Keyword: strings.TrimSpace(q.Keyword),
		Limit: pageSize, Offset: (page - 1) * pageSize,
	}
	rows, err := s.media.List(ctx, filter)
	if err != nil {
		return nil, 0, wrapExecError(err, "list execution media failed")
	}
	total, err := s.media.Count(ctx, filter)
	if err != nil {
		return nil, 0, wrapExecError(err, "count execution media failed")
	}
	out := make([]MediumDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toMediumDTO(row))
	}
	return out, total, nil
}

func (s *MediumService) Get(ctx context.Context, mediumID string) (*MediumDTO, error) {
	if s == nil || s.media == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "medium service is not enabled")
	}
	m, err := s.media.GetByID(ctx, mediumID)
	if err != nil {
		return nil, wrapExecError(err, "get execution medium failed")
	}
	return toMediumDTO(*m), nil
}

func (s *MediumService) recordAudit(ctx context.Context, userID string, action AuditAction, resourceID string, payload map[string]any) {
	_ = s.audit.Record(ctx, AuditRecord{
		ResourceType: "execution_medium", ResourceID: resourceID, Action: action, UserID: userID, Payload: payload,
	})
}

func toMediumDTO(m domain.ExecutionMedium) *MediumDTO {
	return &MediumDTO{
		MediumID: m.MediumID, Name: m.Name, MediumType: string(m.MediumType),
		Environment: m.Environment, Region: m.Region, NetworkZone: m.NetworkZone,
		Capabilities: m.Capabilities, AllowedCommandIDs: m.AllowedCommandIDs,
		MaxRiskLevel: string(m.MaxRiskLevel), Enabled: m.Enabled, HealthStatus: string(m.HealthStatus),
		Description: m.Description, CreatedAt: unixOrZero(m.CreatedAt), UpdatedAt: unixOrZero(m.UpdatedAt),
	}
}

func cloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}
