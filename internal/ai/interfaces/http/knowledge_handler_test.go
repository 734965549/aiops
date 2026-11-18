package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	aiapp "github.com/734965549/aiops/internal/ai/application"
	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
	"github.com/gin-gonic/gin"
)

type fakeKnowledgeUseCase struct {
	listInput         aiapp.KnowledgeListInput
	createInput       aiapp.KnowledgeItemInput
	updateID          string
	updateInput       aiapp.KnowledgeItemInput
	reviewID          string
	reviewAct         string
	deleteID          string
	aggregateAppID    string
	aggregateMinCases int
	aggregateErr      error
	actor             string
	listErr           error
	createErr         error
	updateErr         error
	reviewErr         error
	deleteErr         error
}

func (f *fakeKnowledgeUseCase) List(_ context.Context, in aiapp.KnowledgeListInput) ([]domain.KnowledgeItem, error) {
	f.listInput = in
	if f.listErr != nil {
		return nil, f.listErr
	}
	return []domain.KnowledgeItem{{
		ItemID: "item-1", Scope: domain.KnowledgeScopeGlobal, Title: "标题", Content: "内容",
		Origin: domain.KnowledgeOriginManual, Status: domain.KnowledgeStatusDraft,
	}}, nil
}

func (f *fakeKnowledgeUseCase) Create(_ context.Context, actor aiapp.KnowledgeActor, in aiapp.KnowledgeItemInput) (*domain.KnowledgeItem, error) {
	f.actor = actor.UserID
	f.createInput = in
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &domain.KnowledgeItem{ItemID: "item-new", Status: domain.KnowledgeStatusDraft, Title: in.Title}, nil
}

func (f *fakeKnowledgeUseCase) Update(_ context.Context, actor aiapp.KnowledgeActor, itemID string, in aiapp.KnowledgeItemInput) (*domain.KnowledgeItem, error) {
	f.actor = actor.UserID
	f.updateID = itemID
	f.updateInput = in
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &domain.KnowledgeItem{ItemID: itemID, Status: domain.KnowledgeStatusDraft, Title: in.Title}, nil
}

func (f *fakeKnowledgeUseCase) Review(_ context.Context, actor aiapp.KnowledgeActor, itemID, action string) (*domain.KnowledgeItem, error) {
	f.actor = actor.UserID
	f.reviewID = itemID
	f.reviewAct = action
	if f.reviewErr != nil {
		return nil, f.reviewErr
	}
	return &domain.KnowledgeItem{ItemID: itemID, Status: domain.KnowledgeStatusActive}, nil
}

func (f *fakeKnowledgeUseCase) Delete(_ context.Context, actor aiapp.KnowledgeActor, itemID string) error {
	f.actor = actor.UserID
	f.deleteID = itemID
	return f.deleteErr
}

func (f *fakeKnowledgeUseCase) AggregateFromFeedback(_ context.Context, actor aiapp.KnowledgeActor, applicationID string, minCases int) (*aiapp.AggregateResult, error) {
	f.actor = actor.UserID
	f.aggregateAppID = applicationID
	f.aggregateMinCases = minCases
	if f.aggregateErr != nil {
		return nil, f.aggregateErr
	}
	return &aiapp.AggregateResult{
		Action:      aiapp.AggregateActionCreated,
		SourceCases: 3,
		Item:        &domain.KnowledgeItem{ItemID: "agg-app-1", Status: domain.KnowledgeStatusDraft},
	}, nil
}

func newKnowledgeTestContext(method, path string, body any, params gin.Params, userID string) (*gin.Context, *httptest.ResponseRecorder) {
	return newChatHandlerTestContext(method, path, body, params, userID)
}

func TestKnowledgeHandlerList(t *testing.T) {
	usecase := &fakeKnowledgeUseCase{}
	h := NewKnowledgeHandler(usecase)
	c, w := newKnowledgeTestContext(http.MethodGet, "/ai/knowledge/items?status=draft&limit=5", nil, nil, "admin")

	h.List(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.listInput.Status != "draft" || usecase.listInput.Limit != 5 {
		t.Fatalf("unexpected input: %+v", usecase.listInput)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"item_id":"item-1"`)) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestKnowledgeHandlerListRejectsBadLimit(t *testing.T) {
	h := NewKnowledgeHandler(&fakeKnowledgeUseCase{})
	for _, limit := range []string{"0", "101", "abc"} {
		c, w := newKnowledgeTestContext(http.MethodGet, "/ai/knowledge/items?limit="+limit, nil, nil, "admin")
		h.List(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("limit %s: expected 400, got %d", limit, w.Code)
		}
	}
}

func TestKnowledgeHandlerCreateUpdateReviewDelete(t *testing.T) {
	usecase := &fakeKnowledgeUseCase{}
	h := NewKnowledgeHandler(usecase)
	params := gin.Params{{Key: "item_id", Value: "item-1"}}
	body := map[string]any{"scope": "application", "scope_ref": "app-1", "title": "标题", "content": "内容"}

	c, w := newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/items", body, nil, "admin")
	h.Create(c)
	if w.Code != http.StatusOK || usecase.createInput.ScopeRef != "app-1" || usecase.actor != "admin" {
		t.Fatalf("unexpected create: %d %s %+v", w.Code, w.Body.String(), usecase.createInput)
	}

	c, w = newKnowledgeTestContext(http.MethodPut, "/ai/knowledge/items/item-1", body, params, "admin")
	h.Update(c)
	if w.Code != http.StatusOK || usecase.updateID != "item-1" || usecase.updateInput.Title != "标题" {
		t.Fatalf("unexpected update: %d %s", w.Code, w.Body.String())
	}

	c, w = newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/items/item-1/review", map[string]any{"action": "activate"}, params, "admin")
	h.Review(c)
	if w.Code != http.StatusOK || usecase.reviewAct != "activate" || usecase.reviewID != "item-1" {
		t.Fatalf("unexpected review: %d %s", w.Code, w.Body.String())
	}

	c, w = newKnowledgeTestContext(http.MethodDelete, "/ai/knowledge/items/item-1", nil, params, "admin")
	h.Delete(c)
	if w.Code != http.StatusOK || usecase.deleteID != "item-1" || !bytes.Contains(w.Body.Bytes(), []byte(`"deleted":true`)) {
		t.Fatalf("unexpected delete: %d %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeHandlerReviewRequiresAction(t *testing.T) {
	h := NewKnowledgeHandler(&fakeKnowledgeUseCase{})
	c, w := newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/items/item-1/review", map[string]any{}, gin.Params{{Key: "item_id", Value: "item-1"}}, "admin")
	h.Review(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeHandlerMapsErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", apperr.New(apperr.CodeNotFound, "knowledge item not found"), http.StatusNotFound},
		{"invalid", apperr.New(apperr.CodeInvalidArgument, "invalid knowledge item"), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usecase := &fakeKnowledgeUseCase{reviewErr: tc.err, deleteErr: tc.err, createErr: tc.err}
			h := NewKnowledgeHandler(usecase)
			params := gin.Params{{Key: "item_id", Value: "item-1"}}

			c, w := newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/items/item-1/review", map[string]any{"action": "activate"}, params, "admin")
			h.Review(c)
			if w.Code != tc.code {
				t.Fatalf("review: expected %d, got %d", tc.code, w.Code)
			}

			c, w = newKnowledgeTestContext(http.MethodDelete, "/ai/knowledge/items/item-1", nil, params, "admin")
			h.Delete(c)
			if w.Code != tc.code {
				t.Fatalf("delete: expected %d, got %d", tc.code, w.Code)
			}

			c, w = newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/items", map[string]any{"scope": "global", "title": "t", "content": "c"}, nil, "admin")
			h.Create(c)
			if w.Code != tc.code {
				t.Fatalf("create: expected %d, got %d", tc.code, w.Code)
			}
		})
	}
}

func TestKnowledgeHandlerUnavailable(t *testing.T) {
	h := NewKnowledgeHandler(nil)
	params := gin.Params{{Key: "item_id", Value: "item-1"}}
	for name, call := range map[string]func(*gin.Context){
		"list":   h.List,
		"create": h.Create,
		"update": h.Update,
		"review": h.Review,
		"delete": h.Delete,
	} {
		c, w := newKnowledgeTestContext(http.MethodGet, "/ai/knowledge/items", map[string]any{}, params, "admin")
		call(c)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", name, w.Code)
		}
	}
}

func TestKnowledgeHandlerAggregate(t *testing.T) {
	usecase := &fakeKnowledgeUseCase{}
	h := NewKnowledgeHandler(usecase)
	c, w := newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/aggregate", map[string]any{
		"application_id": "app-1", "min_cases": 3,
	}, nil, "admin")

	h.Aggregate(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if usecase.aggregateAppID != "app-1" || usecase.aggregateMinCases != 3 || usecase.actor != "admin" {
		t.Fatalf("unexpected aggregate input: app=%q min=%d actor=%q", usecase.aggregateAppID, usecase.aggregateMinCases, usecase.actor)
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"action":"created"`)) || !bytes.Contains([]byte(body), []byte(`"source_cases":3`)) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestKnowledgeHandlerAggregateRequiresApplication(t *testing.T) {
	h := NewKnowledgeHandler(&fakeKnowledgeUseCase{})
	c, w := newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/aggregate", map[string]any{}, nil, "admin")
	h.Aggregate(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeHandlerAggregateUnavailable(t *testing.T) {
	h := NewKnowledgeHandler(nil)
	c, w := newKnowledgeTestContext(http.MethodPost, "/ai/knowledge/aggregate", map[string]any{"application_id": "app-1"}, nil, "admin")
	h.Aggregate(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestKnowledgeHandlerResponseShape(t *testing.T) {
	usecase := &fakeKnowledgeUseCase{}
	h := NewKnowledgeHandler(usecase)
	c, w := newKnowledgeTestContext(http.MethodGet, "/ai/knowledge/items", nil, nil, "admin")
	h.List(c)
	var body struct {
		Code string `json:"code"`
		Data struct {
			Items []struct {
				ItemID string `json:"item_id"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "OK" || len(body.Data.Items) != 1 || body.Data.Items[0].Status != "draft" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}
