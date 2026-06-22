package domain

import "time"

// LeaseStatus 租约状态。
type LeaseStatus string

const (
	LeaseActive   LeaseStatus = "active"
	LeaseReleased LeaseStatus = "released"
	LeaseExpired  LeaseStatus = "expired"
)

// ExecutionLease 执行租约。
type ExecutionLease struct {
	LeaseID    string
	TaskID     string
	StepID     string
	AgentID    string
	MediumID   string
	Status     LeaseStatus
	ExpiresAt  time.Time
	ReleasedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LogStreamEntry 执行日志流条目。
type LogStreamEntry struct {
	LogID      string
	LeaseID    string
	TaskID     string
	StepID     string
	AgentID    string
	Stream     string
	Sequence   int
	Content    string
	Truncated  bool
	Redacted   bool
	ObservedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
