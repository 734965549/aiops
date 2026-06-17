package application

import (
	"context"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/alert/domain"
	apperr "github.com/734965549/aiops/pkg/errors"
)

const maxCommentLen = 2000
const maxSilenceDuration = 30 * 24 * time.Hour

// AlertListQuery 告警列表查询。
type AlertListQuery struct {
	Page           int
	PageSize       int
	Status         string
	Severity       string
	Source         string
	SourceID       string
	BusinessLine   string
	Environment    string
	ApplicationID  string
	ResourceID     string
	AssigneeUserID string
	Keyword        string
	ActiveOnly     bool
	From           int64
	To             int64
}

// Actor 用户操作者信息。
type Actor struct {
	UserID      string
	DisplayName string
}

// AlertService 告警查询与人工处理状态流转（ops/alert-contract.md §1、§8）。
//
// 覆盖：列表/详情/时间线、认领/转派/开始处理/恢复/关闭/静默/取消静默/备注；
// 每次动作写入 AlertEvent，AuditRecorder 预留审计写入点（§9.4）。
type AlertService struct {
	alerts   domain.AlertRepository
	events   domain.AlertEventRepository
	silences domain.AlertSilenceRepository
	audit    AuditRecorder
}

// NewAlertService 构造告警服务；audit 为 nil 时使用 NoopAuditRecorder。
func NewAlertService(
	alerts domain.AlertRepository,
	events domain.AlertEventRepository,
	silences domain.AlertSilenceRepository,
	audit AuditRecorder,
) *AlertService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AlertService{alerts: alerts, events: events, silences: silences, audit: audit}
}

// List 分页查询告警列表。
func (s *AlertService) List(ctx context.Context, q AlertListQuery) ([]AlertDTO, int64, error) {
	if s == nil || s.alerts == nil {
		return nil, 0, apperr.New(apperr.CodeUnavailable, "alert service is not enabled")
	}
	filter, err := buildAlertFilter(q)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.alerts.List(ctx, filter)
	if err != nil {
		return nil, 0, wrapAlertOpError(err, "list alerts failed")
	}
	total, err := s.alerts.Count(ctx, filter)
	if err != nil {
		return nil, 0, wrapAlertOpError(err, "count alerts failed")
	}
	out := make([]AlertDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToAlertDTO(row))
	}
	return out, total, nil
}

// GetDetail 返回告警详情、时间线（升序）及 related 占位对象。
func (s *AlertService) GetDetail(ctx context.Context, alertID string) (*AlertDetailDTO, error) {
	if s == nil || s.alerts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert service is not enabled")
	}
	alert, err := s.alerts.GetByID(ctx, strings.TrimSpace(alertID))
	if err != nil {
		return nil, wrapAlertOpError(err, "load alert failed")
	}
	eventDTOs := []AlertEventDTO{}
	if s.events != nil {
		events, err := s.events.ListByAlertID(ctx, alert.ID)
		if err != nil {
			return nil, wrapAlertOpError(err, "list alert events failed")
		}
		eventDTOs = make([]AlertEventDTO, 0, len(events))
		for _, e := range events {
			eventDTOs = append(eventDTOs, ToAlertEventDTO(e))
		}
	}
	return &AlertDetailDTO{
		Alert:   ToAlertDTO(*alert),
		Events:  eventDTOs,
		Related: map[string]any{},
	}, nil
}

// Acknowledge 认领告警：new → acknowledged。
func (s *AlertService) Acknowledge(ctx context.Context, alertID string, actor Actor, message string) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanAcknowledge(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot transition from %s to acknowledged", alert.Status)
	}
	now := time.Now()
	alert.Status = domain.StatusAcknowledged
	alert.AcknowledgedAt = &now
	alert.AssigneeUserID = strings.TrimSpace(actor.UserID)
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert failed")
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "告警已认领"
	}
	if err := s.recordEvent(ctx, alert.ID, domain.EventAcknowledged, domain.ActorUser, actor.UserID, actor.DisplayName, msg, nil); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditAcknowledge, map[string]any{"message": msg})
	out := ToAlertDTO(*alert)
	return &out, nil
}

// Assign 转派处理人，非 closed 状态均可转派。
func (s *AlertService) Assign(ctx context.Context, alertID string, actor Actor, assigneeUserID, message string) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanAssign(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot assign from %s", alert.Status)
	}
	assigneeUserID = strings.TrimSpace(assigneeUserID)
	if assigneeUserID == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "assignee_user_id is required")
	}
	alert.AssigneeUserID = assigneeUserID
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert assignment failed")
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "告警已转派"
	}
	payload := map[string]any{"assignee_user_id": assigneeUserID}
	if err := s.recordEvent(ctx, alert.ID, domain.EventAssigned, domain.ActorUser, actor.UserID, actor.DisplayName, msg, payload); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditAssign, payload)
	out := ToAlertDTO(*alert)
	return &out, nil
}

// StartProcessing 开始处理：acknowledged → processing。
func (s *AlertService) StartProcessing(ctx context.Context, alertID string, actor Actor) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanStartProcessing(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot transition from %s to processing", alert.Status)
	}
	alert.Status = domain.StatusProcessing
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert status failed")
	}
	if err := s.recordEvent(ctx, alert.ID, domain.EventProcessingStarted, domain.ActorUser, actor.UserID, actor.DisplayName, "开始处理告警", nil); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditStartProcessing, nil)
	out := ToAlertDTO(*alert)
	return &out, nil
}

// Recover 手动标记恢复 → recovered。
func (s *AlertService) Recover(ctx context.Context, alertID string, actor Actor, message string) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanRecover(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot transition from %s to recovered", alert.Status)
	}
	now := time.Now()
	alert.Status = domain.StatusRecovered
	alert.RecoveredAt = &now
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert status failed")
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "告警已恢复"
	}
	if err := s.recordEvent(ctx, alert.ID, domain.EventRecovered, domain.ActorUser, actor.UserID, actor.DisplayName, msg, nil); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditRecover, map[string]any{"message": msg})
	out := ToAlertDTO(*alert)
	return &out, nil
}

// Close 关闭告警 → closed（最终态）。
func (s *AlertService) Close(ctx context.Context, alertID string, actor Actor, resolution string) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanClose(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot transition from %s to closed", alert.Status)
	}
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "resolution is required")
	}
	now := time.Now()
	alert.Status = domain.StatusClosed
	alert.ClosedAt = &now
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert status failed")
	}
	payload := map[string]any{"resolution": resolution}
	if err := s.recordEvent(ctx, alert.ID, domain.EventClosed, domain.ActorUser, actor.UserID, actor.DisplayName, resolution, payload); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditClose, payload)
	out := ToAlertDTO(*alert)
	return &out, nil
}

// Silence 静默告警并写入 alert_silence 记录；duration_s 最大 30 天。
func (s *AlertService) Silence(ctx context.Context, alertID string, actor Actor, reason string, durationS int64) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanSilence(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot transition from %s to silenced", alert.Status)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "reason is required")
	}
	if durationS <= 0 {
		return nil, apperr.New(apperr.CodeInvalidArgument, "duration_s must be greater than 0")
	}
	duration := time.Duration(durationS) * time.Second
	if duration > maxSilenceDuration {
		return nil, apperr.New(apperr.CodeInvalidArgument, "duration_s exceeds maximum 30d")
	}
	now := time.Now()
	endsAt := now.Add(duration)
	alert.Status = domain.StatusSilenced
	alert.SilencedUntil = &endsAt
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert status failed")
	}
	if s.silences != nil {
		silence := &domain.AlertSilence{
			ID:        newEventID(),
			AlertID:   alert.ID,
			Reason:    reason,
			StartsAt:  now,
			EndsAt:    endsAt,
			CreatedBy: actor.UserID,
		}
		if err := s.silences.Create(ctx, silence); err != nil {
			return nil, wrapAlertOpError(err, "create alert silence failed")
		}
	}
	payload := map[string]any{"reason": reason, "duration_s": durationS, "ends_at": endsAt.Unix()}
	if err := s.recordEvent(ctx, alert.ID, domain.EventSilenced, domain.ActorUser, actor.UserID, actor.DisplayName, reason, payload); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditSilence, payload)
	out := ToAlertDTO(*alert)
	return &out, nil
}

// Unsilence 取消静默：silenced → new。
func (s *AlertService) Unsilence(ctx context.Context, alertID string, actor Actor) (*AlertDTO, error) {
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanUnsilence(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status cannot transition from %s to new", alert.Status)
	}
	alert.Status = domain.StatusNew
	alert.SilencedUntil = nil
	if err := s.alerts.Update(ctx, alert); err != nil {
		return nil, wrapAlertOpError(err, "update alert status failed")
	}
	if err := s.recordEvent(ctx, alert.ID, domain.EventUnsilenced, domain.ActorUser, actor.UserID, actor.DisplayName, "取消静默", nil); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditUnsilence, nil)
	out := ToAlertDTO(*alert)
	return &out, nil
}

// Comment 添加备注，写入 commented 事件（§8.10）。
func (s *AlertService) Comment(ctx context.Context, alertID string, actor Actor, message string) (*AlertEventDTO, error) {
	if s == nil || s.events == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert event service is not enabled")
	}
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanComment(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status %s cannot accept comments", alert.Status)
	}
	message = limitString(strings.TrimSpace(message), maxCommentLen)
	if message == "" {
		return nil, apperr.New(apperr.CodeInvalidArgument, "message is required")
	}
	if err := s.recordEvent(ctx, alert.ID, domain.EventCommented, domain.ActorUser, actor.UserID, actor.DisplayName, message, nil); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditComment, map[string]any{"message": message})
	events, err := s.events.ListByAlertID(ctx, alert.ID)
	if err != nil {
		return nil, wrapAlertOpError(err, "list alert events failed")
	}
	if len(events) == 0 {
		return nil, apperr.New(apperr.CodeInternal, "comment event not found")
	}
	last := events[len(events)-1]
	dto := ToAlertEventDTO(last)
	return &dto, nil
}

// AIAnalysisInput AI 分析入口参数（§9.2）；alert_id 由路径提供。
type AIAnalysisInput struct {
	TimeRange      string
	IncludeLogs    bool
	IncludeMetrics bool
	IncludeChanges bool
}

// RecordExecutionTimelineEvent 供 Execution 模块回写告警时间线（ops/execution-contract.md §7）。
func (s *AlertService) RecordExecutionTimelineEvent(ctx context.Context, alertID string, eventType domain.AlertEventType, actor Actor, message string, payload map[string]any) error {
	if s == nil || s.events == nil {
		return apperr.New(apperr.CodeUnavailable, "alert event service is not enabled")
	}
	if !eventType.IsValid() {
		return apperr.New(apperr.CodeInvalidArgument, "invalid alert event type")
	}
	alert, err := s.alerts.GetByID(ctx, strings.TrimSpace(alertID))
	if err != nil {
		return wrapAlertOpError(err, "load alert failed")
	}
	if err := s.recordEvent(ctx, alert.ID, eventType, domain.ActorUser, actor.UserID, actor.DisplayName, message, payload); err != nil {
		return err
	}
	return nil
}

// RequestAIAnalysis 记录 AI 分析入口动作，写入 ai_analysis_requested 事件（§9.2）。
// 实际分析由前端再调 AI 模块 POST /api/ai/analyze-alert；closed 告警不可发起。
func (s *AlertService) RequestAIAnalysis(ctx context.Context, alertID string, actor Actor, in AIAnalysisInput) (*AlertEventDTO, error) {
	if s == nil || s.events == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert event service is not enabled")
	}
	alert, err := s.getMutableAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}
	if !domain.CanRequestAIAnalysis(alert.Status) {
		return nil, apperr.Newf(apperr.CodeInvalidArgument, "alert status %s cannot request ai analysis", alert.Status)
	}
	timeRange := strings.TrimSpace(in.TimeRange)
	if timeRange == "" {
		timeRange = "30m"
	}
	payload := map[string]any{
		"time_range":      timeRange,
		"include_logs":    in.IncludeLogs,
		"include_metrics": in.IncludeMetrics,
		"include_changes": in.IncludeChanges,
	}
	msg := "发起 AI 分析"
	if err := s.recordEvent(ctx, alert.ID, domain.EventAIAnalysisRequested, domain.ActorUser, actor.UserID, actor.DisplayName, msg, payload); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, alert.ID, actor.UserID, AuditAIAnalysis, payload)
	events, err := s.events.ListByAlertID(ctx, alert.ID)
	if err != nil {
		return nil, wrapAlertOpError(err, "list alert events failed")
	}
	if len(events) == 0 {
		return nil, apperr.New(apperr.CodeInternal, "ai analysis event not found")
	}
	last := events[len(events)-1]
	dto := ToAlertEventDTO(last)
	return &dto, nil
}

func (s *AlertService) getMutableAlert(ctx context.Context, alertID string) (*domain.Alert, error) {
	if s == nil || s.alerts == nil {
		return nil, apperr.New(apperr.CodeUnavailable, "alert service is not enabled")
	}
	alert, err := s.alerts.GetByID(ctx, strings.TrimSpace(alertID))
	if err != nil {
		return nil, wrapAlertOpError(err, "load alert failed")
	}
	return alert, nil
}

func buildAlertFilter(q AlertListQuery) (domain.AlertFilter, error) {
	statuses, err := parseStatusFilter(q.Status)
	if err != nil {
		return domain.AlertFilter{}, err
	}
	severities, err := parseSeverityFilter(q.Severity)
	if err != nil {
		return domain.AlertFilter{}, err
	}
	filter := domain.AlertFilter{
		Source:         strings.TrimSpace(q.Source),
		SourceID:       strings.TrimSpace(q.SourceID),
		BusinessLine:   strings.TrimSpace(q.BusinessLine),
		Environment:    strings.TrimSpace(q.Environment),
		ApplicationID:  strings.TrimSpace(q.ApplicationID),
		ResourceID:     strings.TrimSpace(q.ResourceID),
		AssigneeUserID: strings.TrimSpace(q.AssigneeUserID),
		Keyword:        strings.TrimSpace(q.Keyword),
		ActiveOnly:     q.ActiveOnly,
		Statuses:       statuses,
		Severities:     severities,
		Limit:          q.PageSize,
		Offset:         (q.Page - 1) * q.PageSize,
	}
	if q.From > 0 {
		t := time.Unix(q.From, 0)
		filter.FromFirstSeen = &t
	}
	if q.To > 0 {
		t := time.Unix(q.To, 0)
		filter.ToFirstSeen = &t
	}
	return filter, nil
}

// parseStatusFilter 解析列表 status 过滤；支持逗号分隔，大小写不敏感。
func parseStatusFilter(raw string) ([]domain.AlertStatus, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]domain.AlertStatus, 0, len(parts))
	for _, p := range parts {
		st := domain.AlertStatus(strings.ToLower(p))
		if !st.IsValid() {
			return nil, apperr.Newf(apperr.CodeInvalidArgument, "invalid status filter: %s", p)
		}
		out = append(out, st)
	}
	return out, nil
}

// parseSeverityFilter 解析列表 severity 过滤；P1 等展示值会先转小写再校验。
func parseSeverityFilter(raw string) ([]domain.AlertSeverity, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]domain.AlertSeverity, 0, len(parts))
	for _, p := range parts {
		sev := domain.AlertSeverity(strings.ToLower(p))
		if !sev.IsValid() {
			return nil, apperr.Newf(apperr.CodeInvalidArgument, "invalid severity filter: %s", p)
		}
		out = append(out, sev)
	}
	return out, nil
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
