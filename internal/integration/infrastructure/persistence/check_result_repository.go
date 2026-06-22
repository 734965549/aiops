package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/integration/domain"
	"github.com/734965549/aiops/pkg/database"
	"gorm.io/gorm"
)

type checkResultModel struct {
	database.BaseModel
	CheckID      string    `gorm:"column:check_id;type:varchar(64);uniqueIndex;not null"`
	AccountID    string    `gorm:"column:account_id;type:varchar(64);not null;index:idx_integration_check_account_checked,priority:1"`
	Status       string    `gorm:"column:status;type:varchar(16);not null"`
	Message      string    `gorm:"column:message;type:varchar(512);not null;default:''"`
	Capabilities []byte    `gorm:"column:capabilities;type:jsonb;not null"`
	CheckedAt    time.Time `gorm:"column:checked_at;not null;index:idx_integration_check_account_checked,priority:2"`
}

func (checkResultModel) TableName() string { return "integration_check_result" }

type CheckResultRepository struct {
	db *gorm.DB
}

func NewCheckResultRepository(db *gorm.DB) *CheckResultRepository {
	return &CheckResultRepository{db: db}
}

func (r *CheckResultRepository) Create(ctx context.Context, check *domain.ConnectivityCheck) error {
	if r == nil || r.db == nil {
		return errors.New("integration check repository is not configured")
	}
	if check == nil {
		return errors.New("connectivity check is nil")
	}
	caps := make([]string, 0, len(check.Capabilities))
	for _, c := range check.Capabilities {
		caps = append(caps, string(c))
	}
	capJSON, err := marshalCapabilities(caps)
	if err != nil {
		return err
	}
	checkedAt := check.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	m := checkResultModel{
		CheckID: check.CheckID, AccountID: check.AccountID, Status: string(check.Status),
		Message: check.Message, Capabilities: capJSON, CheckedAt: checkedAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return database.MapUniqueViolation(err, domain.ErrAlreadyExists)
	}
	check.CheckedAt = checkedAt
	return nil
}

func (r *CheckResultRepository) LatestByAccountID(ctx context.Context, accountID string) (*domain.ConnectivityCheck, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("integration check repository is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, domain.ErrNotFound
	}
	var row checkResultModel
	if err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Order("checked_at DESC, id DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	acc, err := r.loadAccountProvider(ctx, accountID)
	if err != nil {
		acc = ""
	}
	out := toCheckDomain(&row, domain.ProviderType(acc))
	return &out, nil
}

func (r *CheckResultRepository) loadAccountProvider(ctx context.Context, accountID string) (string, error) {
	var row accountModel
	if err := r.db.WithContext(ctx).Select("provider").Where("account_id = ?", accountID).First(&row).Error; err != nil {
		return "", err
	}
	return row.Provider, nil
}

func toCheckDomain(row *checkResultModel, provider domain.ProviderType) domain.ConnectivityCheck {
	caps := unmarshalCapabilities(row.Capabilities)
	outCaps := make([]domain.Capability, 0, len(caps))
	for _, c := range caps {
		outCaps = append(outCaps, domain.Capability(c))
	}
	return domain.ConnectivityCheck{
		CheckID: row.CheckID, AccountID: row.AccountID, Status: domain.ConnectivityStatus(row.Status),
		Provider: provider, Capabilities: outCaps, Message: row.Message, CheckedAt: row.CheckedAt,
	}
}
