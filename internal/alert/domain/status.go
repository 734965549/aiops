package domain

// AlertStatus 告警处理状态（ops/alert-contract.md §4.2）。
type AlertStatus string

const (
	// StatusNew 新建：新告警或恢复后再次触发，未有人认领。
	StatusNew AlertStatus = "new"
	// StatusAcknowledged 已认领：已有人接手，但未开始处理。
	StatusAcknowledged AlertStatus = "acknowledged"
	// StatusProcessing 处理中：已进入处理过程。
	StatusProcessing AlertStatus = "processing"
	// StatusRecovered 已恢复：外部系统已恢复，等待确认关闭。
	StatusRecovered AlertStatus = "recovered"
	// StatusClosed 已关闭：告警处理完毕，最终态。
	StatusClosed AlertStatus = "closed"
	// StatusSilenced 已静默：暂时不提醒，但仍可继续接收外部更新。
	StatusSilenced AlertStatus = "silenced"
)

// IsValid 判断是否为平台定义的告警状态。
func (s AlertStatus) IsValid() bool {
	switch s {
	case StatusNew, StatusAcknowledged, StatusProcessing, StatusRecovered, StatusClosed, StatusSilenced:
		return true
	default:
		return false
	}
}

// IsTerminal 返回是否为最终态；closed 后同一 dedup_key 再次 firing 应创建新生命周期记录（§4.2 约束）。
func (s AlertStatus) IsTerminal() bool {
	return s == StatusClosed
}

// IsActive 返回告警是否仍处于未关闭状态（含 silenced，静默期间仍更新 last_seen_at、occurrence_count）。
func (s AlertStatus) IsActive() bool {
	return s != StatusClosed
}

// DisplayLabel 返回前端展示文案（新建、已认领等）。
func (s AlertStatus) DisplayLabel() string {
	switch s {
	case StatusNew:
		return "新建"
	case StatusAcknowledged:
		return "已认领"
	case StatusProcessing:
		return "处理中"
	case StatusRecovered:
		return "已恢复"
	case StatusClosed:
		return "已关闭"
	case StatusSilenced:
		return "已静默"
	default:
		return string(s)
	}
}
