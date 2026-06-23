package domain

import (
	"fmt"
	"time"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSuccess   RunStatus = "success"
	RunStatusPartial   RunStatus = "partial"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type TriggerType string

const (
	TriggerManual    TriggerType = "manual"
	TriggerScheduled TriggerType = "scheduled"
)

// TimelineEvent 运行时间线事件（可审计摘要）。
type TimelineEvent struct {
	TS     int64  `json:"ts"`
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
}

// InspectionRun 一次巡检执行。
type InspectionRun struct {
	RunID       string
	PolicyID    string
	Status      RunStatus
	TriggerType TriggerType
	Summary     string
	Timeline    []TimelineEvent
	StartedAt   *time.Time
	FinishedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *InspectionRun) CanTransitionTo(next RunStatus) bool {
	if r == nil {
		return false
	}
	allowed := map[RunStatus][]RunStatus{
		RunStatusPending:   {RunStatusRunning, RunStatusCancelled},
		RunStatusRunning:   {RunStatusSuccess, RunStatusPartial, RunStatusFailed, RunStatusCancelled},
		RunStatusSuccess:   {},
		RunStatusPartial:   {},
		RunStatusFailed:    {},
		RunStatusCancelled: {},
	}
	for _, s := range allowed[r.Status] {
		if s == next {
			return true
		}
	}
	return false
}

func (r *InspectionRun) TransitionTo(next RunStatus) error {
	if !r.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, r.Status, next)
	}
	now := time.Now().UTC()
	switch next {
	case RunStatusRunning:
		r.StartedAt = &now
	case RunStatusSuccess, RunStatusPartial, RunStatusFailed, RunStatusCancelled:
		r.FinishedAt = &now
	}
	r.Status = next
	return nil
}

func (r *InspectionRun) AppendTimeline(event, detail string) {
	r.Timeline = append(r.Timeline, TimelineEvent{
		TS:     time.Now().UTC().Unix(),
		Event:  event,
		Detail: detail,
	})
}
