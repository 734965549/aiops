package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

// fakeKnowledgeRepo 是内存版知识仓储，行为与 GORM 实现保持同一口径。
type fakeKnowledgeRepo struct {
	items     map[string]domain.KnowledgeItem
	getErr    error
	listErr   error
	updateErr error
	// filterCalls 记录注入取数次数，验证 chat 只取一次。
	filterCalls int
}

func newFakeKnowledgeRepo(items ...domain.KnowledgeItem) *fakeKnowledgeRepo {
	repo := &fakeKnowledgeRepo{items: map[string]domain.KnowledgeItem{}}
	for _, item := range items {
		repo.items[item.ItemID] = item
	}
	return repo
}

func (f *fakeKnowledgeRepo) Create(_ context.Context, item domain.KnowledgeItem) error {
	f.items[item.ItemID] = item
	return nil
}

func (f *fakeKnowledgeRepo) Get(_ context.Context, itemID string) (*domain.KnowledgeItem, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	item, ok := f.items[itemID]
	if !ok {
		return nil, domain.ErrKnowledgeItemNotFound
	}
	copied := item
	return &copied, nil
}

func (f *fakeKnowledgeRepo) Update(_ context.Context, item domain.KnowledgeItem) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if _, ok := f.items[item.ItemID]; !ok {
		return domain.ErrKnowledgeItemNotFound
	}
	f.items[item.ItemID] = item
	return nil
}

func (f *fakeKnowledgeRepo) Delete(_ context.Context, itemID string) error {
	if _, ok := f.items[itemID]; !ok {
		return domain.ErrKnowledgeItemNotFound
	}
	delete(f.items, itemID)
	return nil
}

func (f *fakeKnowledgeRepo) List(_ context.Context, filter domain.KnowledgeItemFilter) ([]domain.KnowledgeItem, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]domain.KnowledgeItem, 0, len(f.items))
	for _, item := range f.items {
		if filter.Status != nil && item.Status != *filter.Status {
			continue
		}
		if filter.Scope != nil && item.Scope != *filter.Scope {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (f *fakeKnowledgeRepo) ListActiveForTarget(_ context.Context, applicationID, resourceType string) ([]domain.KnowledgeItem, error) {
	f.filterCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]domain.KnowledgeItem, 0, len(f.items))
	for _, item := range f.items {
		if item.Status != domain.KnowledgeStatusActive {
			continue
		}
		if !item.MatchesTarget(applicationID, resourceType) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (f *fakeKnowledgeRepo) FindByScope(_ context.Context, scope domain.KnowledgeScope, scopeRef string, origin domain.KnowledgeOrigin) (*domain.KnowledgeItem, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, item := range f.items {
		if item.Scope == scope && item.ScopeRef == scopeRef && item.Origin == origin {
			copied := item
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeKnowledgeRepo) AddHit(_ context.Context, itemIDs []string, updatedAt time.Time) error {
	for _, id := range itemIDs {
		if item, ok := f.items[id]; ok {
			item.HitCount++
			item.UpdatedAt = updatedAt
			f.items[id] = item
		}
	}
	return nil
}

func (f *fakeKnowledgeRepo) AddUseful(_ context.Context, itemIDs []string, updatedAt time.Time) error {
	for _, id := range itemIDs {
		if item, ok := f.items[id]; ok {
			item.UsefulCount++
			item.UpdatedAt = updatedAt
			f.items[id] = item
		}
	}
	return nil
}

func knowledgeTestItem(id string, status domain.KnowledgeStatus) domain.KnowledgeItem {
	return domain.KnowledgeItem{
		ItemID: id, Scope: domain.KnowledgeScopeGlobal, Title: "发布导致 CPU 升高",
		Content: "先查发布记录", Origin: domain.KnowledgeOriginManual, Status: status,
	}
}

func TestKnowledgeServiceCreateRequiresReview(t *testing.T) {
	repo := newFakeKnowledgeRepo()
	audit := &capturingAIAudit{}
	svc := NewKnowledgeService(repo, audit)

	item, err := svc.Create(context.Background(), KnowledgeActor{UserID: "admin"}, KnowledgeItemInput{
		Scope: "application", ScopeRef: "app-1", Title: "  发布导致 CPU 升高 ", Content: "先查发布记录",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Status != domain.KnowledgeStatusDraft || item.Origin != domain.KnowledgeOriginManual {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.Title != "发布导致 CPU 升高" || item.CreatedBy != "admin" {
		t.Fatalf("unexpected normalize: %+v", item)
	}
	// draft 不注入。
	injection := svc.Injection(context.Background(), "app-1", "")
	if len(injection.Items) != 0 || injection.Status != KnowledgeStatusEmpty {
		t.Fatalf("draft must not be injected: %+v", injection)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditKnowledgeEdit {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}

	// 激活后注入，并带回 item_id 与附加段。
	reviewed, err := svc.Review(context.Background(), KnowledgeActor{UserID: "reviewer"}, item.ItemID, "activate")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if reviewed.Status != domain.KnowledgeStatusActive || reviewed.ReviewedBy != "reviewer" || reviewed.ReviewedAt == nil {
		t.Fatalf("unexpected reviewed item: %+v", reviewed)
	}
	injection = svc.Injection(context.Background(), "app-1", "")
	if injection.Status != KnowledgeStatusOK || len(injection.Items) != 1 || injection.Items[0].ItemID != item.ItemID {
		t.Fatalf("active item must be injected: %+v", injection)
	}
	if injection.Appendix == "" {
		t.Fatal("appendix must not be empty")
	}
	// 其他应用不命中 application scope 条目。
	if other := svc.Injection(context.Background(), "app-2", ""); len(other.Items) != 0 {
		t.Fatalf("scope leak: %+v", other.Items)
	}
	if len(audit.rows) != 2 || audit.rows[1].Action != AuditKnowledgeReview {
		t.Fatalf("unexpected review audit: %+v", audit.rows)
	}
}

func TestKnowledgeServiceUpdateResetsActiveToDraft(t *testing.T) {
	active := knowledgeTestItem("item-1", domain.KnowledgeStatusActive)
	repo := newFakeKnowledgeRepo(active)
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})

	reviewed := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	active.ReviewedBy = "reviewer"
	active.ReviewedAt = &reviewed
	repo.items[active.ItemID] = active

	updated, err := svc.Update(context.Background(), KnowledgeActor{UserID: "admin"}, active.ItemID, KnowledgeItemInput{
		Scope: "global", Title: "新标题", Content: "新内容",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	// 生效内容被改动必须重新审核。
	if updated.Status != domain.KnowledgeStatusDraft || updated.ReviewedBy != "" || updated.ReviewedAt != nil {
		t.Fatalf("active item must return to draft after edit: %+v", updated)
	}
	if injection := svc.Injection(context.Background(), "", ""); len(injection.Items) != 0 {
		t.Fatalf("edited item must not be injected before review: %+v", injection.Items)
	}
}

func TestKnowledgeServiceReviewDisable(t *testing.T) {
	repo := newFakeKnowledgeRepo(knowledgeTestItem("item-1", domain.KnowledgeStatusActive))
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})

	if _, err := svc.Review(context.Background(), KnowledgeActor{UserID: "reviewer"}, "item-1", "disable"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if injection := svc.Injection(context.Background(), "", ""); len(injection.Items) != 0 {
		t.Fatalf("disabled item must not be injected: %+v", injection.Items)
	}
	if _, err := svc.Review(context.Background(), KnowledgeActor{UserID: "reviewer"}, "item-1", "explode"); apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if _, err := svc.Review(context.Background(), KnowledgeActor{UserID: "reviewer"}, "missing", "activate"); apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestKnowledgeServiceValidatesInput(t *testing.T) {
	svc := NewKnowledgeService(newFakeKnowledgeRepo(), NoopAuditRecorder{})
	cases := []KnowledgeItemInput{
		{Scope: "team", Title: "t", Content: "c"},
		{Scope: "global", Title: "", Content: "c"},
		{Scope: "application", Title: "t", Content: "c"},
		{Scope: "global", ScopeRef: "app-1", Title: "t", Content: "c"},
	}
	for _, in := range cases {
		if _, err := svc.Create(context.Background(), KnowledgeActor{UserID: "admin"}, in); apperr.FromError(err).Code != apperr.CodeInvalidArgument {
			t.Fatalf("expected invalid argument for %+v, got %v", in, err)
		}
	}
}

func TestKnowledgeServiceHitAndUsefulAccounting(t *testing.T) {
	repo := newFakeKnowledgeRepo(knowledgeTestItem("item-1", domain.KnowledgeStatusActive))
	audit := &capturingAIAudit{}
	svc := NewKnowledgeService(repo, audit)

	svc.RecordInjection(context.Background(), []string{"item-1", "item-1", ""})
	if repo.items["item-1"].HitCount != 1 {
		t.Fatalf("hit_count = %d", repo.items["item-1"].HitCount)
	}
	svc.RecordUsageByFeedback(context.Background(), []string{"item-1"}, true)
	if repo.items["item-1"].UsefulCount != 1 {
		t.Fatalf("useful_count = %d", repo.items["item-1"].UsefulCount)
	}
	if repo.items["item-1"].Status != domain.KnowledgeStatusActive {
		t.Fatal("useful item must stay active")
	}
}

func TestKnowledgeServiceAutoDisablesInefficientItem(t *testing.T) {
	item := knowledgeTestItem("item-1", domain.KnowledgeStatusActive)
	item.HitCount = domain.KnowledgeAutoDisableMinHits - 1
	repo := newFakeKnowledgeRepo(item)
	audit := &capturingAIAudit{}
	svc := NewKnowledgeService(repo, audit)

	svc.RecordInjection(context.Background(), []string{"item-1"})
	if repo.items["item-1"].Status != domain.KnowledgeStatusActive {
		t.Fatal("item below threshold must stay active")
	}
	// 达到阈值仍未有效 → 自动停用并写审计。
	svc.RecordUsageByFeedback(context.Background(), []string{"item-1"}, false)
	if repo.items["item-1"].Status != domain.KnowledgeStatusDisabled {
		t.Fatalf("expected auto disabled, got %s", repo.items["item-1"].Status)
	}
	found := false
	for _, row := range audit.rows {
		if row.Action == AuditKnowledgeReview && row.Payload["action"] == "auto_disable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("auto disable audit missing: %+v", audit.rows)
	}
	if injection := svc.Injection(context.Background(), "", ""); len(injection.Items) != 0 {
		t.Fatalf("auto disabled item must not be injected: %+v", injection.Items)
	}
}

func TestKnowledgeServiceListFilters(t *testing.T) {
	repo := newFakeKnowledgeRepo(
		knowledgeTestItem("draft-1", domain.KnowledgeStatusDraft),
		knowledgeTestItem("active-1", domain.KnowledgeStatusActive),
	)
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})

	all, err := svc.List(context.Background(), KnowledgeListInput{})
	if err != nil || len(all) != 2 {
		t.Fatalf("list all = %d, err=%v", len(all), err)
	}
	active, err := svc.List(context.Background(), KnowledgeListInput{Status: "active"})
	if err != nil || len(active) != 1 || active[0].ItemID != "active-1" {
		t.Fatalf("list active = %+v, err=%v", active, err)
	}
	if _, err := svc.List(context.Background(), KnowledgeListInput{Status: "bogus"}); apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	if _, err := svc.List(context.Background(), KnowledgeListInput{Scope: "bogus"}); apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestKnowledgeServiceDegradesOnRepoError(t *testing.T) {
	repo := newFakeKnowledgeRepo()
	repo.listErr = errors.New("db down")
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})

	injection := svc.Injection(context.Background(), "app-1", "")
	if injection.Status != KnowledgeStatusError || len(injection.Items) != 0 {
		t.Fatalf("expected error status, got %+v", injection)
	}
	if disabled := NewKnowledgeService(nil, nil); disabled.Injection(context.Background(), "", "").Status != KnowledgeStatusSkipped {
		t.Fatal("disabled service must report skipped")
	}
}

// fakeMemorySource 提供聚合用的记忆案例。
type fakeMemorySource struct {
	cases []domain.MemoryCase
	err   error
}

func (f *fakeMemorySource) ListForAggregation(_ context.Context, _, _ string, limit int) ([]domain.MemoryCase, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := f.cases
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func aggregateTestCases(count int) []domain.MemoryCase {
	out := make([]domain.MemoryCase, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, domain.MemoryCase{
			CaseID: "case-" + string(rune('a'+i)), UserID: "admin", ApplicationID: "app-1",
			Summary: "发布后 CPU 升高，回滚后恢复", Recommendations: []string{"先查发布记录", "确认实例规格"},
			Status: domain.MemoryCaseStatusActive, QualityScore: 1,
		})
	}
	return out
}

func TestKnowledgeAggregateCreatesDraft(t *testing.T) {
	repo := newFakeKnowledgeRepo()
	audit := &capturingAIAudit{}
	svc := NewKnowledgeService(repo, audit)
	svc.SetMemorySource(&fakeMemorySource{cases: aggregateTestCases(3)})

	result, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 0)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if result.Action != AggregateActionCreated || result.SourceCases != 3 || result.Item == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	item := result.Item
	if item.Status != domain.KnowledgeStatusDraft || item.Origin != domain.KnowledgeOriginFeedbackAggregate {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.Scope != domain.KnowledgeScopeApplication || item.ScopeRef != "app-1" {
		t.Fatalf("unexpected scope: %+v", item)
	}
	if !strings.Contains(item.Content, "发布后 CPU 升高") || !strings.Contains(item.Content, "先查发布记录") {
		t.Fatalf("content not composed from cases: %s", item.Content)
	}
	// draft 不注入。
	if injection := svc.Injection(context.Background(), "app-1", ""); len(injection.Items) != 0 {
		t.Fatalf("aggregated draft must not be injected: %+v", injection.Items)
	}
	if len(audit.rows) != 1 || audit.rows[0].Payload["action"] != "aggregate" {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}

func TestKnowledgeAggregateIsIdempotent(t *testing.T) {
	repo := newFakeKnowledgeRepo()
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})
	source := &fakeMemorySource{cases: aggregateTestCases(3)}
	svc.SetMemorySource(source)

	first, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 0)
	if err != nil {
		t.Fatalf("first aggregate: %v", err)
	}
	// 新增案例后再次聚合：同一 item_id 被更新，不产生第二条。
	source.cases = append(source.cases, domain.MemoryCase{
		CaseID: "case-z", UserID: "admin", ApplicationID: "app-1", Summary: "扩容后恢复",
		Status: domain.MemoryCaseStatusActive, QualityScore: 2,
	})
	second, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 0)
	if err != nil {
		t.Fatalf("second aggregate: %v", err)
	}
	if second.Action != AggregateActionUpdated || second.Item.ItemID != first.Item.ItemID {
		t.Fatalf("expected update of the same item, got %+v", second)
	}
	if len(repo.items) != 1 {
		t.Fatalf("aggregate must keep a single item, got %d", len(repo.items))
	}
	if !strings.Contains(repo.items[first.Item.ItemID].Content, "扩容后恢复") {
		t.Fatalf("content not refreshed: %s", repo.items[first.Item.ItemID].Content)
	}
}

func TestKnowledgeAggregateSkipsActiveItem(t *testing.T) {
	repo := newFakeKnowledgeRepo()
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})
	svc.SetMemorySource(&fakeMemorySource{cases: aggregateTestCases(3)})

	created, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 0)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	// 管理员激活聚合结果后，机器不得覆盖线上知识。
	if _, err := svc.Review(context.Background(), KnowledgeActor{UserID: "reviewer"}, created.Item.ItemID, "activate"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	before := repo.items[created.Item.ItemID].Content
	again, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 0)
	if err != nil {
		t.Fatalf("re-aggregate: %v", err)
	}
	if again.Action != AggregateActionSkippedActive {
		t.Fatalf("expected skipped_active, got %s", again.Action)
	}
	if repo.items[created.Item.ItemID].Content != before {
		t.Fatal("active item must not be overwritten by aggregation")
	}
}

func TestKnowledgeAggregateNeedsEnoughCases(t *testing.T) {
	repo := newFakeKnowledgeRepo()
	svc := NewKnowledgeService(repo, NoopAuditRecorder{})
	svc.SetMemorySource(&fakeMemorySource{cases: aggregateTestCases(2)})

	result, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 3)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if result.Action != AggregateActionInsufficientCases || result.Item != nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(repo.items) != 0 {
		t.Fatalf("no draft should be created, got %d", len(repo.items))
	}
}

func TestKnowledgeAggregateValidatesInput(t *testing.T) {
	svc := NewKnowledgeService(newFakeKnowledgeRepo(), NoopAuditRecorder{})
	if _, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "", 0); apperr.FromError(err).Code != apperr.CodeFailedPrecondition {
		t.Fatalf("expected failed precondition without memory source, got %v", err)
	}
	svc.SetMemorySource(&fakeMemorySource{})
	if _, err := svc.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "  ", 0); apperr.FromError(err).Code != apperr.CodeInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
	failing := NewKnowledgeService(newFakeKnowledgeRepo(), NoopAuditRecorder{})
	failing.SetMemorySource(&fakeMemorySource{err: errors.New("db down")})
	if _, err := failing.AggregateFromFeedback(context.Background(), KnowledgeActor{UserID: "admin"}, "app-1", 0); apperr.FromError(err).Code != apperr.CodeInternal {
		t.Fatalf("expected internal error, got %v", err)
	}
}

func TestKnowledgeServiceDelete(t *testing.T) {
	repo := newFakeKnowledgeRepo(knowledgeTestItem("item-1", domain.KnowledgeStatusDraft))
	audit := &capturingAIAudit{}
	svc := NewKnowledgeService(repo, audit)

	if err := svc.Delete(context.Background(), KnowledgeActor{UserID: "admin"}, "item-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := repo.items["item-1"]; ok {
		t.Fatal("item not deleted")
	}
	if err := svc.Delete(context.Background(), KnowledgeActor{UserID: "admin"}, "item-1"); apperr.FromError(err).Code != apperr.CodeNotFound {
		t.Fatalf("expected not found on repeat delete, got %v", err)
	}
	if len(audit.rows) != 1 || audit.rows[0].Payload["action"] != "delete" {
		t.Fatalf("unexpected audit: %+v", audit.rows)
	}
}
