// Package domain 定义平台操作审计事件目录（各模块写入 action 字段）。
package domain

// 认证审计（iam_auth_audit 表，由 Identity 模块写入）：
//   - login / refresh / logout
//   - result: success / failure

// 操作审计（audit_operation 表）按 resource_type + action 分类：

// Alert（resource_type=alert）：
//   - acknowledge / assign / start_processing / recover / close
//   - silence / unsilence / comment
//   - ai_analysis_requested
//   - execution_create（告警侧创建执行任务）
//   - source_create / source_update / source_delete（resource_type=alert_source）
//   - ingest（Webhook 接入告警）

// Asset（resource_type=application | resource | match_rule）：
//   - create / update / delete

// Runbook（resource_type=runbook | alert）：
//   - runbook_create / runbook_update / runbook_delete
//   - runbook_enable / runbook_disable
//   - runbook_recommend（resource_type=alert）

// Execution（resource_type=execution | alert）：
//   - create / create_from_runbook / confirm / reject / execute

// AI（resource_type=ai）：
//   - analyze_alert / tool_invoke
