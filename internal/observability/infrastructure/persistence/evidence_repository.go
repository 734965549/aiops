package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	obsapp "github.com/734965549/aiops/internal/observability/application"
	"github.com/734965549/aiops/internal/observability/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type evidenceModel struct {
	database.BaseModel
	EvidenceID      string     `gorm:"column:evidence_id;type:varchar(64);uniqueIndex;not null"`
	TraceID         string     `gorm:"column:trace_id;type:varchar(128);not null;default:''"`
	AccountID       string     `gorm:"column:account_id;type:varchar(64);not null;index"`
	EvidenceType    string     `gorm:"column:evidence_type;type:varchar(32);not null;default:''"`
	Provider        string     `gorm:"column:provider;type:varchar(32);not null;default:''"`
	Collector       string     `gorm:"column:collector;type:varchar(64);not null;default:''"`
	QueryType       string     `gorm:"column:query_type;type:varchar(32);not null"`
	QueryHash       string     `gorm:"column:query_hash;type:varchar(64);not null;default:''"`
	QueryParams     []byte     `gorm:"column:query_params;type:jsonb"`
	ResourceID      string     `gorm:"column:resource_id;type:varchar(64);not null;default:''"`
	CloudResourceID string     `gorm:"column:cloud_resource_id;type:varchar(255);not null;default:''"`
	WindowStart     *time.Time `gorm:"column:window_start"`
	WindowEnd       *time.Time `gorm:"column:window_end"`
	CollectedAt     *time.Time `gorm:"column:collected_at"`
	Status          string     `gorm:"column:status;type:varchar(32);not null;default:'ok'"`
	Reason          string     `gorm:"column:reason;type:varchar(512);not null;default:''"`
	Summary         []byte     `gorm:"column:summary;type:jsonb;not null"`
	Payload         []byte     `gorm:"column:payload;type:jsonb"`
	PayloadHash     string     `gorm:"column:payload_hash;type:varchar(64);not null;default:''"`
	SchemaVersion   string     `gorm:"column:schema_version;type:varchar(64);not null;default:''"`
	Truncated       bool       `gorm:"column:truncated;not null;default:false"`
	OriginalCount   *int       `gorm:"column:original_count"`
}

func (evidenceModel) TableName() string { return "obs_evidence_ref" }

type EvidenceRepository struct {
	db *gorm.DB
}

func NewEvidenceRepository(db *gorm.DB) *EvidenceRepository {
	return &EvidenceRepository{db: db}
}

func (r *EvidenceRepository) Create(ctx context.Context, ref *domain.EvidenceRef) error {
	if r == nil || r.db == nil {
		return errors.New("observability evidence repository is not configured")
	}
	if ref == nil {
		return errors.New("evidence ref is nil")
	}
	if ref.Status == "" {
		ref.Status = "ok"
	}
	if ref.EvidenceType == "" {
		ref.EvidenceType = ref.QueryType
	}
	m, err := toEvidenceModel(ref)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *EvidenceRepository) SaveSnapshot(ctx context.Context, ref *domain.EvidenceRef) error {
	if r == nil || r.db == nil {
		return errors.New("observability evidence repository is not configured")
	}
	if ref == nil || ref.EvidenceID == "" {
		return errors.New("evidence snapshot id is required")
	}
	if len(ref.Payload) == 0 || !json.Valid(ref.Payload) {
		return errors.New("evidence snapshot payload must be valid JSON")
	}
	if ref.SchemaVersion == "" || ref.CollectedAt == nil || ref.Status == "" {
		return errors.New("evidence snapshot metadata is incomplete")
	}
	canonicalInput, err := obsapp.CanonicalJSON(ref.Payload)
	if err != nil {
		return err
	}
	if ref.PayloadHash != obsapp.HashPayload(canonicalInput) {
		return errors.New("evidence snapshot payload hash mismatch")
	}
	var normalized string
	if err := r.db.WithContext(ctx).Raw("SELECT CAST(? AS jsonb)::text", string(ref.Payload)).Scan(&normalized).Error; err != nil {
		return err
	}
	canonicalStored, err := obsapp.CanonicalJSON(json.RawMessage(normalized))
	if err != nil {
		return err
	}
	ref.Payload = json.RawMessage(normalized)
	ref.PayloadHash = obsapp.HashPayload(canonicalStored)
	m, err := toEvidenceModel(ref)
	if err != nil {
		return err
	}
	m.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "evidence_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"summary":        m.Summary,
			"payload":        m.Payload,
			"payload_hash":   m.PayloadHash,
			"schema_version": m.SchemaVersion,
			"truncated":      m.Truncated,
			"original_count": m.OriginalCount,
			"collected_at":   m.CollectedAt,
			"status":         m.Status,
			"reason":         m.Reason,
			"updated_at":     m.UpdatedAt,
		}),
	}).Create(&m).Error
}

func (r *EvidenceRepository) GetByID(ctx context.Context, evidenceID string) (*domain.EvidenceRef, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("observability evidence repository is not configured")
	}
	var m evidenceModel
	if err := r.db.WithContext(ctx).Where("evidence_id = ?", evidenceID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return toDomainEvidence(&m)
}

func toEvidenceModel(ref *domain.EvidenceRef) (*evidenceModel, error) {
	summary, err := marshalEvidenceMap(ref.Summary)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		summary = []byte("{}")
	}
	queryParams, err := marshalEvidenceMap(ref.QueryParams)
	if err != nil {
		return nil, err
	}
	return &evidenceModel{
		EvidenceID: ref.EvidenceID, TraceID: ref.TraceID, AccountID: ref.AccountID,
		EvidenceType: ref.EvidenceType, Provider: ref.Provider, Collector: ref.Collector,
		QueryType: ref.QueryType, QueryHash: ref.QueryHash, QueryParams: queryParams,
		ResourceID: ref.ResourceID, CloudResourceID: ref.CloudResourceID,
		WindowStart: ref.WindowStart, WindowEnd: ref.WindowEnd, CollectedAt: ref.CollectedAt,
		Status: ref.Status, Reason: ref.Reason, Summary: summary, Payload: ref.Payload,
		PayloadHash: ref.PayloadHash, SchemaVersion: ref.SchemaVersion,
		Truncated: ref.Truncated, OriginalCount: ref.OriginalCount,
	}, nil
}

func toDomainEvidence(m *evidenceModel) (*domain.EvidenceRef, error) {
	summary, err := unmarshalEvidenceMap(m.Summary)
	if err != nil {
		return nil, err
	}
	queryParams, err := unmarshalEvidenceMap(m.QueryParams)
	if err != nil {
		return nil, err
	}
	return &domain.EvidenceRef{
		EvidenceID: m.EvidenceID, TraceID: m.TraceID, AccountID: m.AccountID,
		EvidenceType: m.EvidenceType, Provider: m.Provider, Collector: m.Collector,
		QueryType: m.QueryType, QueryHash: m.QueryHash, QueryParams: queryParams,
		ResourceID: m.ResourceID, CloudResourceID: m.CloudResourceID,
		WindowStart: m.WindowStart, WindowEnd: m.WindowEnd, CollectedAt: m.CollectedAt,
		Status: m.Status, Reason: m.Reason, Summary: summary,
		Payload: append(json.RawMessage(nil), m.Payload...), PayloadHash: m.PayloadHash,
		SchemaVersion: m.SchemaVersion, Truncated: m.Truncated, OriginalCount: m.OriginalCount,
	}, nil
}

func marshalEvidenceMap(value map[string]any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func unmarshalEvidenceMap(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
