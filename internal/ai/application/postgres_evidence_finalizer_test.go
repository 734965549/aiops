package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakePostgresEvidenceStore struct {
	saved *PostgresEvidenceResult
	err   error
	calls int
}

func (f *fakePostgresEvidenceStore) SavePostgres(_ context.Context, _ PostgresEvidenceQuery, evidence *PostgresEvidenceResult) error {
	f.saved = clonePostgresEvidence(evidence)
	f.calls++
	return f.err
}

func TestFinalizePostgresEvidenceSortsAndPersistsFinalSnapshot(t *testing.T) {
	store := &fakePostgresEvidenceStore{}
	out := finalizePostgresEvidence(context.Background(), store, PostgresEvidenceQuery{}, &PostgresEvidenceResult{
		Status: EvidenceStatusOK,
		Sessions: []PostgresSession{
			{SessionID: 1, SessionStatus: "active", TransactionExecutionTime: 10},
			{SessionID: 2, SessionStatus: "active", WaitEventType: "Lock", TransactionExecutionTime: 5},
		},
	}, 0)
	if out.Status != EvidenceStatusOK || out.EvidenceID == "" || store.saved == nil {
		t.Fatalf("snapshot was not persisted: %+v", out)
	}
	if len(out.Sessions) != 2 || out.Sessions[0].SessionID != 2 {
		t.Fatalf("lock-wait session must be prioritized: %+v", out.Sessions)
	}
	if out.OriginalCount != 2 || out.RetainedCount != 2 || out.SchemaVersion != PostgresEvidenceSchemaV1 {
		t.Fatalf("unexpected snapshot metadata: %+v", out)
	}
}

func TestFinalizePostgresEvidenceFailsClosedOnPersistError(t *testing.T) {
	store := &fakePostgresEvidenceStore{err: errors.New("db down")}
	out := finalizePostgresEvidence(context.Background(), store, PostgresEvidenceQuery{}, &PostgresEvidenceResult{
		Status: EvidenceStatusOK, Sessions: []PostgresSession{{SessionID: 1}},
	}, 0)
	if out.Status != EvidenceStatusError || out.Reason != "postgres_snapshot_persist_failed" || out.EvidenceID != "" || len(out.Sessions) != 0 {
		t.Fatalf("unpersisted evidence must not support diagnosis: %+v", out)
	}
}

func TestFinalizePostgresEvidenceTruncatesWithoutLosingCounts(t *testing.T) {
	sessions := make([]PostgresSession, 100)
	for i := range sessions {
		sessions[i] = PostgresSession{SessionID: int32(i), AppName: "long-application-name", QueryID: "query-id"}
	}
	store := &fakePostgresEvidenceStore{}
	out := finalizePostgresEvidence(context.Background(), store, PostgresEvidenceQuery{}, &PostgresEvidenceResult{
		Status: EvidenceStatusOK, Sessions: sessions,
	}, 1024)
	if out.Status != EvidenceStatusOK || !out.Truncated || out.Reason != "truncated" {
		t.Fatalf("unexpected truncation result: %+v", out)
	}
	if out.OriginalCount != 100 || out.RetainedCount >= out.OriginalCount || store.saved == nil {
		t.Fatalf("truncation counts missing: %+v", out)
	}
}

func TestBuildPostgresDiagnosisClassifiesEvidence(t *testing.T) {
	cases := []struct {
		name       string
		sessions   []PostgresSession
		wantCode   string
		wantStatus string
	}{
		{name: "lock", sessions: []PostgresSession{{SessionID: 1, WaitEventType: "Lock"}}, wantCode: "lock_wait", wantStatus: "suspected"},
		{name: "idle", sessions: []PostgresSession{{SessionID: 1, SessionStatus: "idle in transaction", TransactionExecutionTime: 60}}, wantCode: "idle_in_transaction", wantStatus: "confirmed"},
		{name: "active", sessions: []PostgresSession{{SessionID: 1, SessionStatus: "active", TransactionExecutionTime: 60}}, wantCode: "active_long_query", wantStatus: "suspected"},
		{name: "empty", sessions: nil, wantCode: "undetermined", wantStatus: "undetermined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := &chatContextData{Alert: longTransactionTestAlert(), Postgres: &PostgresEvidenceResult{
				Status: EvidenceStatusOK, EvidenceID: "ev-pg-1", Sessions: tc.sessions,
			}}
			diagnosis := buildPostgresDiagnosis(data)
			if diagnosis.RootCause.Code != tc.wantCode {
				t.Fatalf("root cause = %q, want %q", diagnosis.RootCause.Code, tc.wantCode)
			}
			if diagnosis.RootCause.Status != tc.wantStatus {
				t.Fatalf("root status = %q, want %q", diagnosis.RootCause.Status, tc.wantStatus)
			}
		})
	}
}

func TestPostgresIdleDiagnosisConfirmsCategoryNotApplicationReason(t *testing.T) {
	diagnosis := buildPostgresDiagnosis(&chatContextData{Alert: longTransactionTestAlert(), Postgres: &PostgresEvidenceResult{
		Status: EvidenceStatusOK, EvidenceID: "ev-pg-idle",
		Sessions: []PostgresSession{{
			SessionID: 42, SessionStatus: "idle in transaction", TransactionExecutionTime: 900,
			QueryID: "query-42", AppName: "payment-worker", WaitEventType: "Client", WaitEventName: "ClientRead",
			SQLOperation: "UPDATE", SQLSummary: "UPDATE orders SET status = ? WHERE id = ?",
		}},
		TopSQL: []PostgresTopSQL{
			{QueryID: "query-42", SQLOperation: "UPDATE", SQLSummary: "UPDATE orders SET status = ? WHERE id = ?"},
			{QueryID: "unrelated-query", SQLOperation: "SELECT", SQLSummary: "SELECT private_value"},
		},
	}})
	if diagnosis.RootCause.Status != "confirmed" || !strings.Contains(diagnosis.RootCause.Statement, "仍未定") {
		t.Fatalf("confirmed category must retain an undetermined application reason: %+v", diagnosis.RootCause)
	}
	if len(diagnosis.MissingEvidence) != 1 || diagnosis.MissingEvidence[0] != "application_logs" {
		t.Fatalf("application log requirement missing: %+v", diagnosis.MissingEvidence)
	}
	facts := postgresDiagnosisEvidenceLines(diagnosis)
	joined := strings.Join(facts, "\n")
	for _, expected := range []string{"不是锁等待", "UPDATE orders", "关联 Top SQL", "query-42"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("diagnosis facts missing %q: %s", expected, joined)
		}
	}
	if strings.Contains(joined, "private_value") || strings.Contains(joined, "unrelated-query") {
		t.Fatalf("unrelated Top SQL entered diagnosis: %s", joined)
	}
}

func TestPostgresLockWithoutBlockedProcessDoesNotClaimBlocker(t *testing.T) {
	diagnosis := buildPostgresDiagnosis(&chatContextData{Alert: longTransactionTestAlert(), Postgres: &PostgresEvidenceResult{
		Status: EvidenceStatusOK, EvidenceID: "ev-pg-lock",
		Sessions: []PostgresSession{{SessionID: 1, WaitEventType: "Lock"}},
	}})
	if diagnosis.RootCause.Code != "lock_wait" {
		t.Fatalf("unexpected root cause: %+v", diagnosis.RootCause)
	}
	if strings.Contains(diagnosis.RootCause.Statement, "直接阻塞者") || strings.Contains(postgresDiagnosisSummary(diagnosis), "直接阻塞者") {
		t.Fatalf("lock statement must not invent a blocker: %+v", diagnosis.RootCause)
	}
}

func TestChatAnswerReplayPreservesDiagnosis(t *testing.T) {
	original := ChatAnswer{
		Summary: "test", Evidence: []string{}, Impact: []string{}, Recommendations: []string{},
		Uncertainties: []string{},
		Diagnosis: &ChatDiagnosis{
			Facts: []ChatDiagnosisFact{}, Hypotheses: []ChatHypothesis{}, MissingEvidence: []string{},
			RootCause: ChatRootCause{Status: "suspected", Code: "lock_wait", Statement: "lock", EvidenceIDs: []string{"ev-1"}},
		},
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := parseChatAnswerForReplay(string(raw))
	if err != nil {
		t.Fatalf("parse replay: %v", err)
	}
	if replayed.Diagnosis == nil || replayed.Diagnosis.RootCause.Code != "lock_wait" || replayed.Diagnosis.Hypotheses == nil {
		t.Fatalf("diagnosis was not preserved: %+v", replayed.Diagnosis)
	}
}

func TestParseChatAnswerIgnoresModelProvidedDiagnosis(t *testing.T) {
	content := `{"summary":"model claim","diagnosis":{"facts":[],"root_cause":{"status":"confirmed","code":"idle_in_transaction","statement":"model confirmed","evidence_ids":[]},"hypotheses":[],"missing_evidence":[]}}`
	answer, err := parseChatAnswer(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if answer.Diagnosis != nil {
		t.Fatalf("model must not inject diagnosis: %+v", answer.Diagnosis)
	}
	replayed, err := parseChatAnswerForReplay(content)
	if err != nil {
		t.Fatalf("parse replay: %v", err)
	}
	if replayed.Diagnosis == nil || replayed.Diagnosis.RootCause.Status != "confirmed" {
		t.Fatalf("stored diagnosis was not preserved: %+v", replayed.Diagnosis)
	}
}

func TestBuildPostgresDiagnosisRequiresLongTransactionContext(t *testing.T) {
	data := &chatContextData{Postgres: &PostgresEvidenceResult{Status: EvidenceStatusError, Reason: "denied"}}
	if diagnosis := buildPostgresDiagnosis(data); diagnosis != nil {
		t.Fatalf("non long-transaction context must not inject diagnosis: %+v", diagnosis)
	}
	data.Alert = longTransactionTestAlert()
	diagnosis := buildPostgresDiagnosis(data)
	if diagnosis == nil || diagnosis.metricConfirmed {
		t.Fatalf("metric anomaly must not be invented: %+v", diagnosis)
	}
	if strings.Contains(postgresDiagnosisSummary(diagnosis), "指标异常已确认") {
		t.Fatalf("summary must not claim metric anomaly without metric evidence: %+v", diagnosis)
	}
}

func TestPostgresDiagnosisJSONUsesEmptyArrays(t *testing.T) {
	diagnosis := buildPostgresDiagnosis(&chatContextData{Alert: longTransactionTestAlert(), Postgres: &PostgresEvidenceResult{
		Status: EvidenceStatusOK, EvidenceID: "ev-pg-1",
		Sessions: []PostgresSession{{SessionID: 1, SessionStatus: "active", TransactionExecutionTime: 60}},
	}})
	raw, err := json.Marshal(diagnosis)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"hypotheses":[]`, `"missing_evidence":[]`} {
		if !strings.Contains(string(raw), fragment) {
			t.Fatalf("diagnosis JSON must use empty arrays instead of null: %s", raw)
		}
	}
}

func TestBuildPostgresDiagnosisFailureIsUndetermined(t *testing.T) {
	diagnosis := buildPostgresDiagnosis(&chatContextData{Alert: longTransactionTestAlert(), Postgres: &PostgresEvidenceResult{
		Status: EvidenceStatusPermissionDenied, Reason: "denied",
	}})
	if diagnosis.RootCause.Status != "undetermined" || len(diagnosis.Hypotheses) != 3 || len(diagnosis.MissingEvidence) == 0 {
		t.Fatalf("failed evidence must produce missing evidence and hypotheses: %+v", diagnosis)
	}
	if got := postgresDiagnosisSummary(diagnosis); got == "" || got == "疑似存在长事务未提交" {
		t.Fatalf("unexpected fallback summary: %q", got)
	}
}
