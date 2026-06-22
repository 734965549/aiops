package persistence

import (
	"context"

	"github.com/734965549/aiops/internal/integration/domain"
	"gorm.io/gorm"
)

// UnitOfWork 在单数据库事务内协调 integration 多表写入。
type UnitOfWork struct {
	db *gorm.DB
}

func NewUnitOfWork(db *gorm.DB) *UnitOfWork {
	return &UnitOfWork{db: db}
}

func (u *UnitOfWork) WithinTransaction(ctx context.Context, fn func(ctx context.Context, repos domain.TransactionRepositories) error) error {
	if u == nil || u.db == nil {
		return gorm.ErrInvalidDB
	}
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repos := domain.TransactionRepositories{
			Accounts:     &AccountRepository{db: tx},
			Credentials:  &CredentialRepository{db: tx},
			Capabilities: &CapabilityRepository{db: tx},
			Checks:       &CheckResultRepository{db: tx},
		}
		return fn(ctx, repos)
	})
}
