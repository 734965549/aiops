package persistence

import (
	"context"
	"errors"

	"github.com/734965549/aiops/internal/inspection/domain"
	"gorm.io/gorm"
)

type ArtifactUnitOfWork struct {
	db *gorm.DB
}

func NewArtifactUnitOfWork(db *gorm.DB) *ArtifactUnitOfWork {
	return &ArtifactUnitOfWork{db: db}
}

func (u *ArtifactUnitOfWork) WithinArtifactsTransaction(ctx context.Context, fn func(context.Context, domain.ArtifactRepositories) error) error {
	if u == nil || u.db == nil {
		return errors.New("inspection artifact unit of work is not configured")
	}
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, domain.ArtifactRepositories{
			Runs:            NewRunRepository(tx),
			Findings:        NewFindingRepository(tx),
			Recommendations: NewRecommendationRepository(tx),
		})
	})
}

var _ domain.ArtifactUnitOfWork = (*ArtifactUnitOfWork)(nil)
