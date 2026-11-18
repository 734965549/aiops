package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

func memoryManagementFixture() *fakeMemoryRepo {
	return &fakeMemoryRepo{cases: []domain.MemoryCase{
		{CaseID: "case-a", UserID: "u-1", ApplicationID: "app-1", Summary: "CPU 持续升高", Keywords: "cpu high", Status: domain.MemoryCaseStatusActive, QualityScore: 2},
		{CaseID: "case-b", UserID: "u-2", ApplicationID: "app-2", Summary: "磁盘写满", Keywords: "disk full", Status: domain.MemoryCaseStatusRetired, QualityScore: -2},
		{CaseID: "case-c", UserID: "u-1", ApplicationID: "app-2", Summary: "内存泄漏", Keywords: "memory leak", Status: domain.MemoryCaseStatusActive, QualityScore: 1},
	}}
}

func TestMemoryServiceListCasesFilters(t *testing.T) {
	repo := memoryManagementFixture()
	svc := NewMemoryService(repo, nil)

	all, err := svc.ListCases(context.Background(), MemoryCaseListInput{})
	if err != nil || len(all) != 3 {
		t.Fatalf("list all: len=%d err=%v", len(all), err)
	}

	active, err := svc.ListCases(context.Background(), MemoryCaseListInput{Status: "active"})
	if err != nil || len(active) != 2 {
		t.Fatalf("list active: len=%d err=%v", len(active), err)
	}
	for _, c := range active {
		if c.Status != domain.MemoryCaseStatusActive {
			t.Fatalf("non-active case returned: %+v", c)
		}
	}

	owned, err := svc.ListCases(context.Background(), MemoryCaseListInput{UserID: "u-1", Status: "active"})
	if err != nil || len(owned) != 2 {
		t.Fatalf("list by user: len=%d err=%v", len(owned), err)
	}

	keyword, err := svc.ListCases(context.Background(), MemoryCaseListInput{Keyword: "磁盘"})
	if err != nil || len(keyword) != 1 || keyword[0].CaseID != "case-b" {
		t.Fatalf("keyword filter: %+v err=%v", keyword, err)
	}
}

func TestMemoryServiceListCasesRejectsInvalidStatus(t *testing.T) {
	svc := NewMemoryService(memoryManagementFixture(), nil)
	_, err := svc.ListCases(context.Background(), MemoryCaseListInput{Status: "deleted"})
	if apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestMemoryServiceListCasesWrapsRepositoryError(t *testing.T) {
	repo := memoryManagementFixture()
	repo.listErr = errors.New("db down")
	svc := NewMemoryService(repo, nil)
	if _, err := svc.ListCases(context.Background(), MemoryCaseListInput{}); apperr.CodeOf(err) != apperr.CodeInternal {
		t.Fatalf("expected internal error, got %v", err)
	}
}

func TestMemoryServiceUpdateCaseStatusRetiresAndAudits(t *testing.T) {
	repo := memoryManagementFixture()
	audit := &capturingAIAudit{}
	svc := NewMemoryService(repo, nil)
	svc.SetAudit(audit)
	fixed := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	updated, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "case-a", "retired")
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if updated.Status != domain.MemoryCaseStatusRetired || !updated.UpdatedAt.Equal(fixed) {
		t.Fatalf("unexpected case: %+v", updated)
	}
	stored, _ := repo.Get(context.Background(), "case-a")
	if stored.Status != domain.MemoryCaseStatusRetired {
		t.Fatalf("repo status = %s", stored.Status)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("expected one audit row, got %d", len(audit.rows))
	}
	row := audit.rows[0]
	if row.Action != AuditMemoryReview || row.UserID != "admin-1" || row.ResourceID != "case-a" {
		t.Fatalf("unexpected audit: %+v", row)
	}
	if row.Payload["previous_status"] != "active" || row.Payload["status"] != "retired" {
		t.Fatalf("unexpected audit payload: %+v", row.Payload)
	}

	// 停用后不再参与回答检索。
	result := svc.SearchSimilar(context.Background(), MemorySearchInput{
		UserID:  "u-1",
		Context: domain.SessionContext{ApplicationID: "app-1"},
	})
	if len(result.Cases) != 0 {
		t.Fatalf("retired case must not be injected: %+v", result.Cases)
	}

	reactivated, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "case-a", "active")
	if err != nil || reactivated.Status != domain.MemoryCaseStatusActive {
		t.Fatalf("reactivate: %+v err=%v", reactivated, err)
	}
	if len(audit.rows) != 2 || audit.rows[1].Payload["previous_status"] != "retired" {
		t.Fatalf("unexpected reactivate audit: %+v", audit.rows)
	}
}

func TestMemoryServiceUpdateCaseStatusNoopDoesNotAudit(t *testing.T) {
	repo := memoryManagementFixture()
	audit := &capturingAIAudit{}
	svc := NewMemoryService(repo, nil)
	svc.SetAudit(audit)

	updated, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "case-a", "active")
	if err != nil || updated.Status != domain.MemoryCaseStatusActive {
		t.Fatalf("noop update: %+v err=%v", updated, err)
	}
	if len(audit.rows) != 0 {
		t.Fatalf("noop update must not write audit: %+v", audit.rows)
	}
}

func TestMemoryServiceUpdateCaseStatusValidation(t *testing.T) {
	svc := NewMemoryService(memoryManagementFixture(), nil)
	if _, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "case-a", "deleted"); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("invalid status: %v", err)
	}
	if _, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "", "active"); apperr.CodeOf(err) != apperr.CodeInvalidArgument {
		t.Fatalf("missing case id: %v", err)
	}
	if _, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "case-missing", "active"); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("missing case: %v", err)
	}
}

func TestMemoryManagementDisabled(t *testing.T) {
	svc := NewMemoryService(nil, nil)
	if _, err := svc.ListCases(context.Background(), MemoryCaseListInput{}); apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("list: %v", err)
	}
	if _, err := svc.UpdateCaseStatus(context.Background(), "admin-1", "case-a", "retired"); apperr.CodeOf(err) != apperr.CodeUnavailable {
		t.Fatalf("update: %v", err)
	}
}
