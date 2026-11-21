package application

import (
	"fmt"
	"strings"
)

type ChatDiagnosisFact struct {
	Statement  string `json:"statement"`
	EvidenceID string `json:"evidence_id,omitempty"`
	Pointer    string `json:"pointer,omitempty"`
}

type ChatRootCause struct {
	Status      string   `json:"status"`
	Code        string   `json:"code"`
	Statement   string   `json:"statement"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type ChatHypothesis struct {
	Code            string   `json:"code"`
	Statement       string   `json:"statement"`
	MissingEvidence []string `json:"missing_evidence"`
}

type ChatDiagnosis struct {
	Facts           []ChatDiagnosisFact `json:"facts"`
	RootCause       ChatRootCause       `json:"root_cause"`
	Hypotheses      []ChatHypothesis    `json:"hypotheses"`
	MissingEvidence []string            `json:"missing_evidence"`
	metricConfirmed bool                `json:"-"`
}

func buildPostgresDiagnosis(data *chatContextData) *ChatDiagnosis {
	if data == nil || (!isPostgresLongTransactionAlert(data.Alert) && !hasPostgresLongTransactionMetric(data.Evidence)) {
		return nil
	}
	diagnosis := &ChatDiagnosis{
		Facts: buildMetricDiagnosisFacts(data.Evidence), Hypotheses: []ChatHypothesis{}, MissingEvidence: []string{},
		metricConfirmed: hasPostgresLongTransactionMetric(data.Evidence),
		RootCause:       ChatRootCause{Status: "undetermined", Code: "undetermined", EvidenceIDs: []string{}},
	}
	activity := data.Postgres
	if activity == nil {
		diagnosis.RootCause.Statement = "根因未定：尚未取得 PostgreSQL 历史会话现场"
		diagnosis.MissingEvidence = []string{"postgres_activity", "postgres_wait_events", "postgres_top_sql"}
		diagnosis.Hypotheses = postgresUndeterminedHypotheses(diagnosis.MissingEvidence)
		return diagnosis
	}
	diagnosis.MissingEvidence = append([]string{}, activity.MissingEvidence...)
	if activity.Status == EvidenceStatusOK && activity.EvidenceID != "" {
		diagnosis.RootCause.EvidenceIDs = []string{activity.EvidenceID}
		for i, session := range activity.Sessions {
			if i >= 6 {
				break
			}
			diagnosis.Facts = append(diagnosis.Facts, ChatDiagnosisFact{
				Statement: postgresSessionFact(session), EvidenceID: activity.EvidenceID, Pointer: fmt.Sprintf("/sessions/%d", i),
			})
		}
	}
	classifyPostgresRootCause(diagnosis, activity)
	if diagnosis.RootCause.Status == "undetermined" {
		diagnosis.Hypotheses = postgresUndeterminedHypotheses(diagnosis.MissingEvidence)
	}
	return diagnosis
}

func hasPostgresLongTransactionMetric(evidence *MetricEvidenceResult) bool {
	if evidence == nil || evidence.Status != EvidenceStatusOK {
		return false
	}
	for _, metric := range evidence.Metrics {
		if !isPostgresLongTransaction(metric.Namespace, metric.Metric) {
			continue
		}
		if metric.First != nil || metric.Peak != nil || metric.Last != nil || len(metric.Points) > 0 || metric.BreachPoints > 0 {
			return true
		}
	}
	return false
}

func buildMetricDiagnosisFacts(evidence *MetricEvidenceResult) []ChatDiagnosisFact {
	if evidence == nil || evidence.Status != EvidenceStatusOK {
		return []ChatDiagnosisFact{}
	}
	facts := make([]ChatDiagnosisFact, 0, len(evidence.Metrics))
	for i, metric := range evidence.Metrics {
		statement := fmt.Sprintf("指标 %s：first=%s%s，peak=%s%s，last=%s%s，破限点 %d，快照保留点 %d",
			metric.Metric,
			formatDiagnosisFloat(metric.First), metric.Unit,
			formatDiagnosisFloat(metric.Peak), metric.Unit,
			formatDiagnosisFloat(metric.Last), metric.Unit,
			metric.BreachPoints, len(metric.Points))
		facts = append(facts, ChatDiagnosisFact{
			Statement: statement, EvidenceID: evidence.EvidenceID, Pointer: fmt.Sprintf("/metrics/%d", i),
		})
	}
	return facts
}

func formatDiagnosisFloat(value *float64) string {
	if value == nil {
		return "无数据"
	}
	return fmt.Sprintf("%g", *value)
}

func postgresSessionFact(session PostgresSession) string {
	parts := []string{fmt.Sprintf("会话 %d 状态 %s", session.SessionID, fallbackDiagnosisText(session.SessionStatus, "未知"))}
	if session.TransactionExecutionTime > 0 {
		parts = append(parts, fmt.Sprintf("事务持续 %d 秒", session.TransactionExecutionTime))
	}
	if session.SampleTime != "" {
		parts = append(parts, "采样时间 "+session.SampleTime)
	}
	if session.AppName != "" {
		parts = append(parts, "应用 "+session.AppName)
	}
	if session.QueryID != "" {
		parts = append(parts, "Query ID "+session.QueryID)
	}
	if session.WaitEventType != "" || session.WaitEventName != "" {
		parts = append(parts, "等待 "+fallbackDiagnosisText(session.WaitEventType, "未知")+"/"+fallbackDiagnosisText(session.WaitEventName, "未知"))
	}
	if blocked := strings.TrimSpace(session.BlockedProcessID); postgresHasBlockedProcess(blocked) {
		parts = append(parts, "阻塞进程 "+blocked)
	}
	return strings.Join(parts, "；")
}

func fallbackDiagnosisText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func classifyPostgresRootCause(diagnosis *ChatDiagnosis, evidence *PostgresEvidenceResult) {
	if evidence.Status != EvidenceStatusOK {
		diagnosis.RootCause.Statement = "根因未定：PostgreSQL 现场采集失败或不可用（" + fallbackDiagnosisText(evidence.Reason, evidence.Status) + "）"
		diagnosis.MissingEvidence = appendMissingEvidence(diagnosis.MissingEvidence, "postgres_activity")
		return
	}
	for _, session := range evidence.Sessions {
		if postgresSessionHasLockWait(session) {
			diagnosis.RootCause.Status = "suspected"
			diagnosis.RootCause.Code = "lock_wait"
			if postgresHasAnyBlockedProcess(evidence.Sessions) {
				diagnosis.RootCause.Statement = "历史会话存在锁等待及直接阻塞者；阻塞链根因仍需人工复核"
			} else {
				diagnosis.RootCause.Statement = "历史会话存在锁等待，但未返回直接阻塞进程；阻塞链根因仍需人工复核"
			}
			return
		}
	}
	for _, session := range evidence.Sessions {
		if strings.EqualFold(session.SessionStatus, "idle in transaction") && session.TransactionExecutionTime > 0 {
			diagnosis.RootCause.Status = "suspected"
			diagnosis.RootCause.Code = "idle_in_transaction"
			diagnosis.RootCause.Statement = "历史会话存在空闲长事务；是否由应用未提交导致需结合应用证据确认"
			return
		}
	}
	for _, session := range evidence.Sessions {
		if strings.EqualFold(session.SessionStatus, "active") && session.TransactionExecutionTime > 0 {
			diagnosis.RootCause.Status = "suspected"
			diagnosis.RootCause.Code = "active_long_query"
			diagnosis.RootCause.Statement = "历史会话存在活跃长 SQL；慢因需结合 Query ID、执行计划和等待事件确认"
			return
		}
	}
	diagnosis.RootCause.Statement = "根因未定：历史窗口内未采到可分类的长事务会话，不能据此反推当时不存在长事务"
}

func postgresUndeterminedHypotheses(missing []string) []ChatHypothesis {
	missing = append([]string{}, missing...)
	return []ChatHypothesis{
		{Code: "idle_in_transaction", Statement: "可能存在空闲事务，尚未证实", MissingEvidence: append([]string{}, missing...)},
		{Code: "active_long_query", Statement: "可能存在活跃长 SQL，尚未证实", MissingEvidence: append([]string{}, missing...)},
		{Code: "lock_wait", Statement: "可能存在锁等待，尚未证实", MissingEvidence: append([]string{}, missing...)},
	}
}

func postgresDiagnosisEvidenceLines(diagnosis *ChatDiagnosis) []string {
	if diagnosis == nil {
		return nil
	}
	out := make([]string, 0, len(diagnosis.Facts)+1)
	for _, fact := range diagnosis.Facts {
		if fact.EvidenceID != "" {
			out = append(out, fact.Statement+"（"+fact.EvidenceID+"）")
		} else {
			out = append(out, fact.Statement)
		}
	}
	if len(out) == 0 {
		out = append(out, diagnosis.RootCause.Statement)
	}
	return out
}

func postgresHasAnyBlockedProcess(sessions []PostgresSession) bool {
	for _, session := range sessions {
		if postgresHasBlockedProcess(session.BlockedProcessID) {
			return true
		}
	}
	return false
}

func postgresDiagnosisSummary(diagnosis *ChatDiagnosis) string {
	if diagnosis == nil {
		return ""
	}
	switch diagnosis.RootCause.Code {
	case "idle_in_transaction":
		return "历史会话显示存在空闲长事务；应用未提交只是待验证方向，不能直接认定为最终根因。"
	case "active_long_query":
		return "历史会话显示存在活跃长 SQL；SQL 为什么慢仍需结合 Query ID 和执行计划确认。"
	case "lock_wait":
		return diagnosis.RootCause.Statement + "。"
	default:
		if diagnosis.metricConfirmed {
			return "长事务指标异常已确认，但当前证据不足，根因未定；空闲事务、慢 SQL 和锁等待均不能直接认定。"
		}
		return "长事务根因未定：当前证据不足以确认指标或会话类型，不能直接认定空闲事务、慢 SQL 或锁等待。"
	}
}

func postgresDiagnosisRecommendations(diagnosis *ChatDiagnosis) []string {
	if diagnosis == nil {
		return nil
	}
	switch diagnosis.RootCause.Code {
	case "idle_in_transaction":
		return []string{"按会话 Query ID 和应用名核对提交/回滚路径；需要终止事务时，只生成待人工确认的处置计划。"}
	case "active_long_query":
		return []string{"按 Query ID 核对执行计划、等待事件和资源瓶颈，不直接认定慢 SQL 的具体原因。"}
	case "lock_wait":
		return []string{"核对 blocked / blocking PID 对应事务，不直接 kill 会话；需要处置时进入 Execution 人工确认。"}
	default:
		return []string{"补齐 RDS 历史会话、等待事件和 Top SQL 证据；证据不足时不直接调整根因相关阈值或执行终止操作。"}
	}
}
