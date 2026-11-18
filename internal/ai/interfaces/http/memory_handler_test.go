package http

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/gin-gonic/gin"
)

type fakeMemoryUseCase struct {
	listInput  aiapp.MemoryCaseListInput
	actor      string
	caseID     string
	target     string
	listErr    error
	updateErr  error
	updateCall bool
}

func (f *fakeMemoryUseCase) ListCases(_ context.Context, in aiapp.MemoryCaseListInput) ([]domain.MemoryCase, error) {
	f.listInput = in
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []domain.MemoryCase{{
		CaseID: "case-1", SourceType: domain.MemoryCaseSourceChatMessage, UserID: "u-1",
		ApplicationID: "app-1", Summary: "CPU 持续升高", Recommendations: []string{"检查负载"},
		RiskLevel: "low", QualityScore: 3, Status: domain.MemoryCaseStatusActive,
	}}, nil
}

func (f *fakeMemoryUseCase) UpdateCaseStatus(_ context.Context, actorID, caseID, status string) (*domain.MemoryCase, error) {
	f.actor = actorID
	f.caseID = caseID
	f.target = status
	f.updateCall = true
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &domain.MemoryCase{CaseID: caseID, Status: domain.MemoryCaseStatusRetired, Summary: "CPU 持续升高"}, nil
}

func TestMemoryHandlerList(t *testing.T) {
	usecase := &fakeMemoryUseCase{}
	h := NewMemoryHandler(usecase)
	c, w := newChatHandlerTestContext(
		http.MethodGet,
		"/ai/memory/cases?status=active&user_id=u-1&keyword=cpu&limit=5&offset=2",
		nil, nil, "admin",
	)

	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	in := usecase.listInput
	if in.Status != "active" || in.UserID != "u-1" || in.Keyword != "cpu" || in.Limit != 5 || in.Offset != 2 {
		t.Fatalf("unexpected input: %+v", in)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"case_id":"case-1"`)) ||
		!bytes.Contains(w.Body.Bytes(), []byte(`"recommendations":["检查负载"]`)) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestMemoryHandlerListRejectsBadPagination(t *testing.T) {
	h := NewMemoryHandler(&fakeMemoryUseCase{})
	for _, path := range []string{
		"/ai/memory/cases?limit=0",
		"/ai/memory/cases?limit=101",
		"/ai/memory/cases?limit=abc",
		"/ai/memory/cases?offset=-1",
		"/ai/memory/cases?offset=abc",
	} {
		c, w := newChatHandlerTestContext(http.MethodGet, path, nil, nil, "admin")
		h.List(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", path, w.Code)
		}
	}
}

func TestMemoryHandlerUpdateStatus(t *testing.T) {
	usecase := &fakeMemoryUseCase{}
	h := NewMemoryHandler(usecase)
	params := gin.Params{{Key: "case_id", Value: "case-1"}}
	c, w := newChatHandlerTestContext(http.MethodPost, "/ai/memory/cases/case-1/status", map[string]any{"status": "retired"}, params, "admin")

	h.UpdateStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !usecase.updateCall || usecase.caseID != "case-1" || usecase.target != "retired" || usecase.actor != "admin" {
		t.Fatalf("unexpected call: %+v", usecase)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"status":"retired"`)) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestMemoryHandlerUpdateStatusRequiresStatus(t *testing.T) {
	h := NewMemoryHandler(&fakeMemoryUseCase{})
	params := gin.Params{{Key: "case_id", Value: "case-1"}}
	c, w := newChatHandlerTestContext(http.MethodPost, "/ai/memory/cases/case-1/status", map[string]any{}, params, "admin")
	h.UpdateStatus(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMemoryHandlerMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", apperr.New(apperr.CodeNotFound, "memory case not found"), http.StatusNotFound},
		{"invalid", apperr.New(apperr.CodeInvalidArgument, "invalid memory case status"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usecase := &fakeMemoryUseCase{updateErr: tc.err}
			h := NewMemoryHandler(usecase)
			params := gin.Params{{Key: "case_id", Value: "case-1"}}
			c, w := newChatHandlerTestContext(http.MethodPost, "/ai/memory/cases/case-1/status", map[string]any{"status": "retired"}, params, "admin")
			h.UpdateStatus(c)
			if w.Code != tc.code {
				t.Fatalf("expected %d, got %d", tc.code, w.Code)
			}
		})
	}
}

func TestMemoryHandlerUnavailable(t *testing.T) {
	h := NewMemoryHandler(nil)
	params := gin.Params{{Key: "case_id", Value: "case-1"}}
	for name, call := range map[string]func(*gin.Context){
		"list":   h.List,
		"update": h.UpdateStatus,
	} {
		c, w := newChatHandlerTestContext(http.MethodGet, "/ai/memory/cases", map[string]any{"status": "retired"}, params, "admin")
		call(c)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", name, w.Code)
		}
	}
}
