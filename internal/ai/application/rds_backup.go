package application

import "context"

// RDS 备份策略与最近备份记录只读查询（ops/ai-contract.md §10.12.1）。
const (
	chatToolRDSBackupQuery = "rds.backup.query"

	// RDSBackupDefaultDays / RDSBackupMaxDays 控制最近备份记录查询窗口。
	RDSBackupDefaultDays = 7
	RDSBackupMaxDays     = 30
	// RDSBackupDefaultLimit / RDSBackupMaxLimit 控制最近备份记录条数。
	RDSBackupDefaultLimit = 10
	RDSBackupMaxLimit     = 20
)

// RDSBackupQuery 是一次 RDS 备份策略/记录查询（provider 无关）。
type RDSBackupQuery struct {
	AccountID  string
	Region     string
	ProjectID  string
	InstanceID string
	// Days 控制最近备份记录的查询窗口，默认 7 天，最大 30 天。
	Days int
	// Limit 控制最近备份记录条数，默认 10，最大 20。
	Limit int
	// Timezone 是备份窗口与记录时间转换使用的本地时区；空值回退 UTC。
	Timezone string
}

// RDSBackupPolicyEvidence 是自动备份策略；StartTimeUTC 是华为云返回的 UTC 时间窗。
type RDSBackupPolicyEvidence struct {
	StartTimeUTC string `json:"start_time_utc,omitempty"`
	WindowLocal  string `json:"window_local,omitempty"`
	Period       string `json:"period,omitempty"`
	KeepDays     int    `json:"keep_days"`
	Timezone     string `json:"timezone,omitempty"`
	Note         string `json:"note,omitempty"`
}

// RDSBackupRecord 是最近一条备份记录，时间已转换为证据时区。
type RDSBackupRecord struct {
	BackupID  string `json:"backup_id"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Status    string `json:"status,omitempty"`
	BeginTime string `json:"begin_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	SizeKB    int64  `json:"size_kb,omitempty"`
}

// RDSBackupResult 是备份策略与最近备份记录；status 非 ok 时 reason 说明原因。
type RDSBackupResult struct {
	Status           string                   `json:"status"`
	Reason           string                   `json:"reason,omitempty"`
	Provider         string                   `json:"provider,omitempty"`
	ProjectID        string                   `json:"project_id,omitempty"`
	InstanceID       string                   `json:"instance_id,omitempty"`
	Timezone         string                   `json:"timezone,omitempty"`
	WindowStart      int64                    `json:"window_start,omitempty"`
	WindowEnd        int64                    `json:"window_end,omitempty"`
	CollectedAt      int64                    `json:"collected_at,omitempty"`
	Policy           *RDSBackupPolicyEvidence `json:"policy,omitempty"`
	Backups          []RDSBackupRecord        `json:"backups,omitempty"`
	BackupTotal      int                      `json:"backup_total,omitempty"`
	CollectionErrors []string                 `json:"collection_errors,omitempty"`
}

// RDSBackupPort 是 RDS 备份只读查询端口；业务失败以 Result.Status 表达。
type RDSBackupPort interface {
	Query(ctx context.Context, userID string, q RDSBackupQuery) (*RDSBackupResult, error)
}

// SetRDSBackups 注入 RDS 备份查询能力；nil 表示关闭该工具。
func (s *ChatService) SetRDSBackups(port RDSBackupPort) {
	if s == nil {
		return
	}
	s.rdsBackups = port
}
