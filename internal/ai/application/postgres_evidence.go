package application

import "context"

const PostgresEvidenceSchemaV1 = "ai.postgres_activity.v1"

// PostgresEvidenceQuery 指定一个已解析 RDS 实例和历史窗口，不允许自由连接地址或 SQL。
type PostgresEvidenceQuery struct {
	AccountID  string `json:"account_id"`
	Region     string `json:"region"`
	ProjectID  string `json:"project_id"`
	InstanceID string `json:"instance_id"`
	ResourceID string `json:"-"`
	From       int64  `json:"from"`
	To         int64  `json:"to"`
	Limit      int    `json:"limit"`
}

type PostgresSession struct {
	SampleTime               string `json:"sample_time,omitempty"`
	BlockedProcessID         string `json:"blocked_process_id,omitempty"`
	DatabaseName             string `json:"database_name,omitempty"`
	SessionID                int32  `json:"session_id"`
	UserHash                 string `json:"user_hash,omitempty"`
	AppName                  string `json:"app_name,omitempty"`
	ClientAddress            string `json:"client_address,omitempty"`
	SessionStartTime         string `json:"session_start_time,omitempty"`
	TransactionStartTime     string `json:"transaction_start_time,omitempty"`
	TransactionExecutionTime int32  `json:"transaction_execution_time,omitempty"`
	QueryStartTime           string `json:"query_start_time,omitempty"`
	StateChangeTime          string `json:"state_change_time,omitempty"`
	WaitEventType            string `json:"wait_event_type,omitempty"`
	WaitEventName            string `json:"wait_event_name,omitempty"`
	SessionStatus            string `json:"session_status,omitempty"`
	QueryID                  string `json:"query_id,omitempty"`
	SQLHash                  string `json:"sql_hash,omitempty"`
	ProcessType              string `json:"process_type,omitempty"`
}

type PostgresWaitEvent struct {
	SampleTime    string `json:"sample_time,omitempty"`
	Count         int32  `json:"count"`
	SessionStatus string `json:"session_status,omitempty"`
	WaitEventType string `json:"wait_event_type,omitempty"`
	WaitEventName string `json:"wait_event_name,omitempty"`
}

type PostgresTopSQL struct {
	SampleTime   string `json:"sample_time,omitempty"`
	Count        int32  `json:"count"`
	DatabaseName string `json:"database_name,omitempty"`
	UserHash     string `json:"user_hash,omitempty"`
	QueryID      string `json:"query_id,omitempty"`
	SQLHash      string `json:"sql_hash,omitempty"`
}

type PostgresEvidenceResult struct {
	Status              string              `json:"status"`
	Reason              string              `json:"reason,omitempty"`
	EvidenceID          string              `json:"evidence_id,omitempty"`
	SchemaVersion       string              `json:"schema_version,omitempty"`
	Provider            string              `json:"provider,omitempty"`
	TraceID             string              `json:"trace_id,omitempty"`
	ProjectID           string              `json:"project_id,omitempty"`
	ResourceID          string              `json:"resource_id,omitempty"`
	InstanceID          string              `json:"instance_id,omitempty"`
	WindowStart         int64               `json:"window_start,omitempty"`
	WindowEnd           int64               `json:"window_end,omitempty"`
	CollectedAt         int64               `json:"collected_at,omitempty"`
	OriginalCount       int                 `json:"original_count"`
	RetainedCount       int                 `json:"retained_count"`
	Truncated           bool                `json:"truncated,omitempty"`
	SessionTotalCount   int32               `json:"session_total_count"`
	WaitEventTotalCount int32               `json:"wait_event_total_count"`
	TopSQLTotalCount    int32               `json:"top_sql_total_count"`
	MissingEvidence     []string            `json:"missing_evidence,omitempty"`
	CollectionErrors    []string            `json:"collection_errors,omitempty"`
	Sessions            []PostgresSession   `json:"sessions,omitempty"`
	WaitEvents          []PostgresWaitEvent `json:"wait_events,omitempty"`
	TopSQL              []PostgresTopSQL    `json:"top_sql,omitempty"`
}

// PostgresEvidencePort 由 RDS/DAS 或固定只读适配器实现，业务失败以 Status 表达。
type PostgresEvidencePort interface {
	Collect(ctx context.Context, userID string, q PostgresEvidenceQuery) (*PostgresEvidenceResult, error)
}

// PostgresEvidenceStore 持久化即将进入诊断和模型输入的现场快照。
type PostgresEvidenceStore interface {
	SavePostgres(ctx context.Context, query PostgresEvidenceQuery, evidence *PostgresEvidenceResult) error
}
