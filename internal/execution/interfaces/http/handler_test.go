package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	execapp "github.com/734965549/aiops/internal/execution/application"
	"github.com/734965549/aiops/internal/execution/domain"
	identityapp "github.com/734965549/aiops/internal/identity/application"
	"github.com/734965549/aiops/internal/server"
	"github.com/734965549/aiops/pkg/auth"
	"github.com/734965549/aiops/pkg/config"
	"github.com/gin-gonic/gin"
)

type fakeExecHTTPAuthorizer struct {
	allowed bool
	last    identityapp.AuthorizationInput
}

func (f *fakeExecHTTPAuthorizer) Authorize(_ context.Context, in identityapp.AuthorizationInput) (*identityapp.AuthorizationResult, error) {
	f.last = in
	return &identityapp.AuthorizationResult{Allowed: f.allowed}, nil
}

type execHTTPFakeTaskRepo struct {
	mu   sync.Mutex
	byID map[string]*domain.Task
}

func newExecHTTPFakeTaskRepo() *execHTTPFakeTaskRepo {
	return &execHTTPFakeTaskRepo{byID: map[string]*domain.Task{}}
}

func (r *execHTTPFakeTaskRepo) Create(_ context.Context, task *domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *task
	r.byID[task.ID] = &cp
	return nil
}

func (r *execHTTPFakeTaskRepo) Update(_ context.Context, task *domain.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[task.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *task
	r.byID[task.ID] = &cp
	return nil
}

func (r *execHTTPFakeTaskRepo) UpdateStatusIf(_ context.Context, taskID string, from, to domain.TaskStatus, mutator func(*domain.Task)) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[taskID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if t.Status != from {
		return nil, domain.ErrInvalidTransition
	}
	cp := *t
	if mutator != nil {
		mutator(&cp)
	}
	cp.Status = to
	r.byID[taskID] = &cp
	out := cp
	return &out, nil
}

func (r *execHTTPFakeTaskRepo) GetByID(_ context.Context, taskID string) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[taskID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *execHTTPFakeTaskRepo) List(_ context.Context, _ domain.TaskFilter) ([]domain.Task, error) {
	return nil, nil
}

func (r *execHTTPFakeTaskRepo) Count(_ context.Context, _ domain.TaskFilter) (int64, error) {
	return 0, nil
}

type execHTTPFakeStepRepo struct {
	mu   sync.Mutex
	rows []domain.Step
}

func (r *execHTTPFakeStepRepo) Create(_ context.Context, step *domain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *step
	r.rows = append(r.rows, cp)
	return nil
}

func (r *execHTTPFakeStepRepo) Update(_ context.Context, step *domain.Step) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == step.ID {
			r.rows[i] = *step
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *execHTTPFakeStepRepo) ListByTaskID(_ context.Context, taskID string) ([]domain.Step, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Step, 0)
	for _, s := range r.rows {
		if s.TaskID == taskID {
			out = append(out, s)
		}
	}
	return out, nil
}

type execHTTPFakeTaskCreator struct {
	tasks *execHTTPFakeTaskRepo
	steps *execHTTPFakeStepRepo
}

func (f *execHTTPFakeTaskCreator) CreateWithSteps(ctx context.Context, task *domain.Task, steps []domain.Step) error {
	if err := domain.ValidateStepsForTask(task, steps); err != nil {
		return err
	}
	if err := f.tasks.Create(ctx, task); err != nil {
		return err
	}
	for i := range steps {
		if err := f.steps.Create(ctx, &steps[i]); err != nil {
			return err
		}
	}
	return nil
}

type execHTTPFakeAlertReader struct{}

func (execHTTPFakeAlertReader) GetForExecution(_ context.Context, _ string) (*execapp.AlertContext, error) {
	return &execapp.AlertContext{
		ID: "a1", Name: "HighCPU", Status: "processing",
		Environment: "prod", ResourceName: "node-1", ResourceType: "host",
	}, nil
}

type execHTTPFakeTimeline struct{}

func (execHTTPFakeTimeline) RecordExecutionCreated(context.Context, string, execapp.Actor, string, map[string]any) error {
	return nil
}
func (execHTTPFakeTimeline) RecordExecutionStarted(context.Context, string, execapp.Actor, string, map[string]any) error {
	return nil
}
func (execHTTPFakeTimeline) RecordExecutionFinished(context.Context, string, execapp.Actor, string, map[string]any) error {
	return nil
}

func newExecutionHTTPEngine(t *testing.T, authz *fakeExecHTTPAuthorizer) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtMgr, err := auth.NewJWTManager(auth.Options{
		Secret: "execution-http-test-secret-length", Issuer: "aiops-test",
		AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt manager: %v", err)
	}
	token, _, err := jwtMgr.IssueAccess(auth.IssueOptions{UserID: "user-1", Username: "alice"})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	taskRepo := newExecHTTPFakeTaskRepo()
	stepRepo := &execHTTPFakeStepRepo{}
	creator := &execHTTPFakeTaskCreator{tasks: taskRepo, steps: stepRepo}
	svc := execapp.NewTaskService(
		taskRepo, stepRepo, creator,
		execHTTPFakeAlertReader{}, execHTTPFakeTimeline{},
		execapp.NoopAuditRecorder{}, nil, nil, nil,
	)

	handler := NewHandler(svc, nil, nil, nil)
	registrar := NewRegistrar(handler, nil, authz)
	engine := server.NewEngine(server.Options{
		Cfg: &config.Config{
			App:      config.AppConfig{Env: "dev", Timezone: "Asia/Shanghai"},
			Server:   config.ServerConfig{Port: 8080},
			Database: config.DatabaseConfig{Host: "127.0.0.1", Name: "aiops", SSLMode: "disable"},
			Auth:     config.AuthConfig{JWTSecret: config.DefaultJWTSecretPlaceholder},
		},
		Authenticator: auth.NewJWTAuthenticator(jwtMgr),
		Registrars:    []server.RouteRegistrar{registrar},
		StartedAt:     time.Now(),
	})
	return engine, token
}

type execAPIEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	TraceID string          `json:"trace_id"`
	Data    json.RawMessage `json:"data"`
}

func decodeExecEnvelope(t *testing.T, body []byte) execAPIEnvelope {
	t.Helper()
	var env execAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, string(body))
	}
	return env
}

func TestCreateTask_Unauthenticated(t *testing.T) {
	engine, _ := newExecutionHTTPEngine(t, &fakeExecHTTPAuthorizer{allowed: true})
	body := `{"source_type":"alert","source_id":"a1","operation_type":"restart"}`
	req := httptest.NewRequest(http.MethodPost, "/api/executions/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeExecEnvelope(t, w.Body.Bytes()).Code != "UNAUTHENTICATED" {
		t.Fatal("expected UNAUTHENTICATED")
	}
}

func TestCreateTask_PermissionDenied(t *testing.T) {
	authz := &fakeExecHTTPAuthorizer{allowed: false}
	engine, token := newExecutionHTTPEngine(t, authz)
	body := `{"source_type":"alert","source_id":"a1","operation_type":"restart"}`
	req := httptest.NewRequest(http.MethodPost, "/api/executions/tasks", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeExecEnvelope(t, w.Body.Bytes()).Code != "PERMISSION_DENIED" {
		t.Fatal("expected PERMISSION_DENIED")
	}
	if authz.last.Resource != "executions" || authz.last.Action != "create" {
		t.Fatalf("unexpected authz: %+v", authz.last)
	}
}

func TestCreateTask_InvalidBody(t *testing.T) {
	engine, token := newExecutionHTTPEngine(t, &fakeExecHTTPAuthorizer{allowed: true})
	req := httptest.NewRequest(http.MethodPost, "/api/executions/tasks", strings.NewReader(`{"source_type":"alert"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if decodeExecEnvelope(t, w.Body.Bytes()).Code != "INVALID_ARGUMENT" {
		t.Fatal("expected INVALID_ARGUMENT")
	}
}

func TestCreateConfirmExecute_Flow(t *testing.T) {
	engine, token := newExecutionHTTPEngine(t, &fakeExecHTTPAuthorizer{allowed: true})

	createBody := `{"source_type":"alert","source_id":"a1","operation_type":"restart"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/executions/tasks", strings.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	engine.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", createW.Code, createW.Body.String())
	}
	createResp := decodeExecEnvelope(t, createW.Body.Bytes())
	if createResp.Code != "OK" || createResp.Message != "ok" {
		t.Fatalf("unexpected create envelope: %+v", createResp)
	}
	var created struct {
		TaskID    string `json:"task_id"`
		Status    string `json:"status"`
		RiskLevel string `json:"risk_level"`
	}
	if err := json.Unmarshal(createResp.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.TaskID == "" || created.Status != "pending_confirm" {
		t.Fatalf("unexpected create data: %+v", created)
	}

	confirmBody := `{"confirm":true,"confirm_text":"CONFIRM"}`
	confirmReq := httptest.NewRequest(http.MethodPost, "/api/executions/tasks/"+created.TaskID+"/confirm", strings.NewReader(confirmBody))
	confirmReq.Header.Set("Authorization", "Bearer "+token)
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmW := httptest.NewRecorder()
	engine.ServeHTTP(confirmW, confirmReq)

	if confirmW.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirmW.Code, confirmW.Body.String())
	}
	var confirmed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(decodeExecEnvelope(t, confirmW.Body.Bytes()).Data, &confirmed); err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "pending_execute" {
		t.Fatalf("expected pending_execute, got %s", confirmed.Status)
	}

	execReq := httptest.NewRequest(http.MethodPost, "/api/executions/tasks/"+created.TaskID+"/execute", strings.NewReader(`{}`))
	execReq.Header.Set("Authorization", "Bearer "+token)
	execReq.Header.Set("Content-Type", "application/json")
	execW := httptest.NewRecorder()
	engine.ServeHTTP(execW, execReq)

	if execW.Code != http.StatusOK {
		t.Fatalf("execute status=%d body=%s", execW.Code, execW.Body.String())
	}
	var detail struct {
		Task struct {
			Status        string `json:"status"`
			ResultSummary string `json:"result_summary"`
		} `json:"task"`
		Steps []struct {
			Status string `json:"status"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(decodeExecEnvelope(t, execW.Body.Bytes()).Data, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != "success" {
		t.Fatalf("expected success, got %s", detail.Task.Status)
	}
	if len(detail.Steps) == 0 {
		t.Fatal("expected at least one step in execute response")
	}
}
