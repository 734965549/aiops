package domain

// AlertEventType 时间线事件类型（ops/alert-contract.md §4.3）。
type AlertEventType string

const (
	// EventTriggered 首次触发。
	EventTriggered AlertEventType = "triggered"
	// EventUpdated 同一告警重复触发或标签/注解更新。
	EventUpdated AlertEventType = "updated"
	// EventRecovered 外部恢复或手动标记恢复。
	EventRecovered AlertEventType = "recovered"
	// EventAcknowledged 用户认领。
	EventAcknowledged AlertEventType = "acknowledged"
	// EventAssigned 转派负责人。
	EventAssigned AlertEventType = "assigned"
	// EventProcessingStarted 开始处理。
	EventProcessingStarted AlertEventType = "processing_started"
	// EventClosed 关闭。
	EventClosed AlertEventType = "closed"
	// EventSilenced 静默。
	EventSilenced AlertEventType = "silenced"
	// EventUnsilenced 取消静默。
	EventUnsilenced AlertEventType = "unsilenced"
	// EventCommented 添加备注。
	EventCommented AlertEventType = "commented"
	// EventAIAnalysisRequested 发起 AI 分析（预留）。
	EventAIAnalysisRequested AlertEventType = "ai_analysis_requested"
	// EventExecutionCreated 基于告警创建执行任务。
	EventExecutionCreated AlertEventType = "execution_created"
	// EventExecutionStarted 执行任务开始运行。
	EventExecutionStarted AlertEventType = "execution_started"
	// EventExecutionFinished 执行任务结束（success/failed）。
	EventExecutionFinished AlertEventType = "execution_finished"
)

// allEventTypes 平台合法事件类型集合。
var allEventTypes = []AlertEventType{
	EventTriggered, EventUpdated, EventRecovered, EventAcknowledged, EventAssigned,
	EventProcessingStarted, EventClosed, EventSilenced, EventUnsilenced, EventCommented,
	EventAIAnalysisRequested, EventExecutionCreated, EventExecutionStarted, EventExecutionFinished,
}

// IsValid 判断是否为平台定义的事件类型。
func (t AlertEventType) IsValid() bool {
	for _, v := range allEventTypes {
		if t == v {
			return true
		}
	}
	return false
}
