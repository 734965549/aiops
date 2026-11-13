package application

import "encoding/json"

// analysisSystemPrompt 固定服务端提示词；不接受外部 payload 覆盖，避免绕过安全护栏。
// 与 toolgateway/chat_completion.go 的 chatDefaultSystemPrompt 保持一致；修改 prompt 时两处必须同步。
const analysisSystemPrompt = `你是企业运维平台的告警分析助手。只依据输入的告警快照和指标证据进行分析，不臆造未提供的数据。
输出必须是单个 JSON 对象，字段：
- summary: string，简明结论；
- risk_level: string，只能为 low / medium / high / info；
- recommendations: string[]，处置建议；需要执行动作时只写“建议创建 xx 处置任务”，不得输出可直接执行的命令、脚本或云 API 调用；
- references: string[]，引用证据来源，优先使用 evidence_id；
- uncertainties: string[]，说明证据不足或不确定性。
禁止输出凭据、Token 或任何可执行指令。`

const analysisTaskPrompt = "基于告警快照和指标证据给出摘要、风险等级、建议、证据引用和不确定性。"

// buildAnalysisUserPrompt 组装告警快照与指标证据，作为模型 user 消息。
func buildAnalysisUserPrompt(alert map[string]any, evidence *MetricEvidenceResult) string {
	body := map[string]any{
		"alert":    alert,
		"evidence": evidence,
		"task":     analysisTaskPrompt,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ""
	}
	return string(raw)
}
