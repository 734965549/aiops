package domain

// 状态机规则见 ops/alert-contract.md §4.2。
// 非法流转由 application 层返回 INVALID_ARGUMENT，不在此包构造 Error 对象。
//
// 人工操作走契约状态图（ActionRecover 仅 processing → recovered）；
// 外部接入 resolved 走 ActionExternalRecover，允许任意 active 态直接进入 recovered（§4.3 recovered 事件）。

// StatusAction 状态流转动作，与契约状态图边标签对齐。
type StatusAction string

const (
	ActionAcknowledge     StatusAction = "acknowledge"      // new → acknowledged
	ActionStartProcessing StatusAction = "start_processing" // acknowledged → processing
	ActionRecover         StatusAction = "recover"          // processing → recovered（人工）
	ActionExternalRecover StatusAction = "external_recover" // active → recovered（外部 resolved）
	ActionClose           StatusAction = "close"            // → closed
	ActionSilence         StatusAction = "silence"          // new/acknowledged/processing → silenced
	ActionUnsilence       StatusAction = "unsilence"        // silenced → new
)

// TransitionStatus 根据当前状态与动作计算目标状态；不允许时第二返回值为 false。
func TransitionStatus(from AlertStatus, action StatusAction) (AlertStatus, bool) {
	switch action {
	case ActionAcknowledge:
		if from == StatusNew {
			return StatusAcknowledged, true
		}
	case ActionStartProcessing:
		if from == StatusAcknowledged {
			return StatusProcessing, true
		}
	case ActionRecover:
		if from == StatusProcessing {
			return StatusRecovered, true
		}
	case ActionExternalRecover:
		switch from {
		case StatusNew, StatusAcknowledged, StatusProcessing, StatusSilenced:
			return StatusRecovered, true
		}
	case ActionClose:
		switch from {
		case StatusNew, StatusAcknowledged, StatusProcessing, StatusRecovered:
			return StatusClosed, true
		}
	case ActionSilence:
		switch from {
		case StatusNew, StatusAcknowledged, StatusProcessing:
			return StatusSilenced, true
		}
	case ActionUnsilence:
		if from == StatusSilenced {
			return StatusNew, true
		}
	}
	return "", false
}

// CanAcknowledge 判断是否允许认领（new → acknowledged）。
func CanAcknowledge(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionAcknowledge)
	return ok
}

// CanStartProcessing 判断是否允许开始处理（acknowledged → processing）。
func CanStartProcessing(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionStartProcessing)
	return ok
}

// CanRecover 判断是否允许手动标记恢复（processing → recovered）。
func CanRecover(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionRecover)
	return ok
}

// CanExternalRecover 判断外部 resolved 是否可将当前 active 告警标记为 recovered。
func CanExternalRecover(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionExternalRecover)
	return ok
}

// CanClose 判断是否允许关闭。
func CanClose(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionClose)
	return ok
}

// CanSilence 判断是否允许静默（new / acknowledged / processing → silenced）。
func CanSilence(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionSilence)
	return ok
}

// CanUnsilence 判断是否允许取消静默（silenced → new）。
func CanUnsilence(status AlertStatus) bool {
	_, ok := TransitionStatus(status, ActionUnsilence)
	return ok
}

// CanAssign 判断是否允许转派（closed 不可转派）。
func CanAssign(status AlertStatus) bool {
	return !status.IsTerminal()
}

// CanComment 判断是否允许添加备注（closed 不可备注）。
func CanComment(status AlertStatus) bool {
	return !status.IsTerminal()
}

// CanRequestAIAnalysis 判断是否允许发起 AI 分析入口（§9.2；closed 不可）。
func CanRequestAIAnalysis(status AlertStatus) bool {
	return !status.IsTerminal()
}
