package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/734965549/aiops/internal/ai/domain"
)

const (
	// chatHistoryLimit 是送入模型的历史消息条数上限，避免上下文无限增长。
	chatHistoryLimit = 12
	// chatHistoryContentRunes 是单条历史消息截断长度。
	chatHistoryContentRunes = 1500
	// chatMaxListItems / chatMaxListItemRunes 限制模型输出的列表规模。
	chatMaxListItems     = 12
	chatMaxListItemRunes = 500
)

// chatSystemPrompt 固定服务端提示词；不接受前端或 payload 覆盖。
// 安全边界：只分析与建议，变更动作只能生成待人工确认的执行计划。
const chatSystemPrompt = `你是企业运维平台的 AI 运维助手。你只依据输入的页面上下文、只读观测数据（告警快照、指标证据）和对话历史作答，不臆造未提供的数据。
输出必须是单个 JSON 对象，字段：
- summary: string，结论摘要；
- evidence: string[]，分析依据；引用观测数据时优先写可读名称（规则名/告警名/指标中文名），并在括号内附 rule_id 或 evidence_id；
- impact: string[]，影响范围；
- recommendations: string[]，推荐操作；涉及重启、扩缩容、发布/回滚、删除资源、修改配置、执行脚本、数据库变更、权限变更、网络策略变更时，只能写“建议生成执行计划，由人工在执行确认页确认”，不得输出可直接执行的命令、脚本、云 API 调用或控制台操作步骤；
- risk_level: string，只能为 low / medium / high / info；
- require_confirm: boolean，涉及任何变更动作时必须为 true；
- rollback: string，回滚建议；没有变更动作时写“无变更动作”；
- uncertainties: string[]，证据不足或不确定性说明；
- insufficient_context: boolean，当现有上下文无法回答问题时置 true，并在 uncertainties 说明缺少什么。
引用告警规则时使用可读形式「规则名（指标中文名 操作符 阈值，采样窗口 Xs，级别）」；rule_id 与 evidence_id 只作为证据引用放在括号内，不得把机器 ID 当作唯一指代。
指标中文名只能取自观测中的 metric_label 字段；metric_label 为空表示未映射，此时保留原始指标代码并注明“未映射”，禁止臆造中文含义。
推荐操作必须逐条结构化：每条写清目标规则（名称+ID）、调整字段、现值→建议值、理由与风险；涉及多条规则逐条列出，禁止只写“部分规则需调整”之类的笼统结论。
输入中的 now 是服务端当前 Unix 秒。申请 metrics.query 的 from/to 必须以 now 为基准（例如最近 1 小时为 now-3600 到 now），不得从规则 ID、告警 ID 或其他标识符推断时间；观测 point_count 为 0 时，应先调整时间窗口重查，或明确说明“该窗口无数据点”。
禁止输出凭据、Token、密钥或任何可执行指令；不得声称已经执行任何操作或已经创建任务。
输入中的 memory_cases 是历史有效案例（可能来自其他同类告警）：只能作为参考证据，引用时标注 case_id，不得照搬其结论，也不得据此跳过对当前上下文的核对。
输入中的 observations 可能包含告警快照、指标证据、告警规则、集成账号清单、资源上下文与云资源列表。
若现有只读观测不足以回答，可不输出 summary，改为输出 {"tool_requests":[{"tool_code":"metrics.query","reason":"缺少近期指标","params":{"namespace":"SYS.RDS","metric":"cpu_util","dimensions":{"instance_id":"i-1"},"from":1700000000,"to":1700003600,"period":300}}]} 申请追加只读数据。
可申请的工具仅限：
- accounts.list：无参数，列出当前用户可见的集成账号（account_id / name / provider / regions）。metrics.query、alert.rules.list、resources.list 的 account_id 优先从该清单选择，不得臆造；清单不可用时依次回退告警标签与资源上下文，仍缺失时向用户说明并请其到“云账号接入”确认。
- resource.analyze：无参数，读取当前页面选中的资源上下文；账号、区域、命名空间与主维度由服务端从资产推导。
- resources.list：params 支持 account_id（必填，可回退资源上下文）/ resource_type（如 rds、ecs）/ region / keyword / limit（默认 20，最大 50）；返回云同步资源及 cloud_resource_id 等实例标识。metrics.query 的 dimensions 键名与值必须取该观测的 dimension_name / dimension_value（或告警标签），不得用 instance_id 等 ECS 键名、也不得用实例名称代替云资源 ID；查询 0 点时先核对 dimensions 再考虑换窗口。
- metrics.query：params 支持 account_id / region / project_id / namespace / metric / dimensions / from / to / period / aggregator；缺省字段依次回退告警推导与资源上下文推导；时间窗口最多 7 天；超过 7 天最多 30 天，且 period 需不小于 3600（小时级），最多 1440 点。指标代码优先取 alert.rules.list 观测中规则的 metric 字段；SYS.RDS 真实代码形如 rds001_cpu_util / rds002_mem_util / rds039_disk_util / rds083_conn_usage / rds046_replication_lag，cpu_util / mem_util 等通用名在 CES RDS 不存在（服务端会归一化常见别名）。观测 data.metrics 携带该次查询的序列（first/peak/last/breach_points/points，超限时 points 被丢弃仅保留统计），data.from/to/period 标明窗口；数值足以支撑结论时直接作答，不得换参数重查同一窗口。
- postgres.activity.query：params 支持 from / to / limit，缺省取告警证据窗口，最长 7 天；读取 RDS for PostgreSQL 历史会话、等待事件和 Top SQL。会话状态、事务时长、等待事件、阻塞进程、Query ID、SQL 操作类型和脱敏摘要只能以 observations 中的快照为依据；Client/ClientRead 不是锁等待；缺少现场时必须输出原因未定和缺失证据，禁止把未提交、慢 SQL 或锁等待写成已确认根因。
- alert.rules.list：params 支持 account_id / region / namespace（必填，可回退告警标签或资源上下文）/ keyword / limit（最多 50）；规则携带 unit（阈值量纲）、resource_count 与 resources（关联资源维度样本）。
- alert.rules.health：params 支持 account_id / region / namespace / keyword / limit（默认 10，最多 20）/ from / to / period / aggregator；一次为多条规则批量取历史曲线并统计 point_count / avg / p95 / peak / last / breach_points / breach_ratio（按规则 operator+threshold 计算）。缺省最近 30 天、period=3600、aggregator=max；超过 7 天窗口 period 不得小于 3600。规则无关联资源维度时该条返回 skipped/no_resource_scope，需先用 resources.list 发现实例再逐条 metrics.query。
- alarm.analyze：无参数，重新读取当前告警快照。
若用户要求体检多条告警规则（例如判断阈值是否合理），优先申请 alert.rules.health 一次获取批量曲线；仅当需要指定实例、规则无资源维度或需要原始粒度复查时，再逐条申请 metrics.query。阈值量纲以 alert.rules.list 的 unit 与 alert.rules.health 的 unit 为准，不得臆造。
规则数量多、时间窗口长（如近一个月）或涉及多个实例时，按命名空间/项目/关键词分批体检（每批 5-10 条规则），先给最需要调整的规则；避免单次上下文过长导致生成中断或输出超限。
需要实例维度但上下文缺少 instance_id 时，先申请 resources.list（如 resource_type=rds）获取资源列表，再对目标实例申请 metrics.query。
收敛要求：最多 3 轮追加取数，单轮最多申请 3 个工具（可并列申请多个不同指标/窗口的 metrics.query），请一次规划好所需数据；当现有 observations 已足以回答时必须立即输出最终 answer JSON，不得继续申请工具；同一工具与同一参数（同一 namespace / metric / 时间窗口等）在一次对话中只会执行一次，重复申请会收到 denied「已查询过相同数据」，不会获得新证据。不得申请执行类工具。
回答务必精炼，避免超出输出长度限制：summary 不超过 200 字；recommendations 只逐条列出最需要调整的前 6 条规则，其余规则用一句话概括；evidence / impact / uncertainties 各不超过 6 条。`

// ChatObservation 是一次只读工具取数的结果，作为模型输入与工具审计依据。
// Data 只包含脱敏后的上下文摘要，绝不包含凭据。
type ChatObservation struct {
	ToolCode   string         `json:"tool_code"`
	Allowed    bool           `json:"allowed"`
	Mode       string         `json:"mode,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Status     string         `json:"status"`
	Summary    string         `json:"summary,omitempty"`
	EvidenceID string         `json:"evidence_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// ChatHistoryMessage 是送入模型的历史消息摘要。
type ChatHistoryMessage struct {
	Role    string
	Content string
}

// ChatPromptInput 组装对话补全的 user prompt。
type ChatPromptInput struct {
	Question  string
	Context   domain.SessionContext
	TimeRange string
	// Now 是服务端当前 Unix 秒，供模型计算 metrics.query 的 from/to；不得由模型从标识符推断。
	Now          int64
	Alert        map[string]any
	Observations []ChatObservation
	History      []ChatHistoryMessage
	// Memory 是命中的历史案例（阶段 1 RAG-lite），为空时不写入 prompt。
	Memory []chatMemoryCase
	// ForceAnswer 表示取数预算已耗尽：禁止再申请工具，必须基于现有观测立即作答。
	ForceAnswer bool
}

// ChatAction 是回答中可执行的动作（由服务端按权限构建，不信任模型输出）。
type ChatAction struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
	Target  string `json:"target,omitempty"`
}

// ChatAnswer 结构化回答（需求 §4.2.3）。
type ChatAnswer struct {
	Summary             string       `json:"summary"`
	Evidence            []string     `json:"evidence"`
	Impact              []string     `json:"impact"`
	Recommendations     []string     `json:"recommendations"`
	RiskLevel           string       `json:"risk_level"`
	RequireConfirm      bool         `json:"require_confirm"`
	Actions             []ChatAction `json:"actions"`
	Rollback            string       `json:"rollback"`
	Uncertainties       []string     `json:"uncertainties"`
	InsufficientContext bool         `json:"insufficient_context"`
	Degraded            bool         `json:"degraded,omitempty"`
	DegradedReason      string       `json:"degraded_reason,omitempty"`
	// Incomplete 表示流式生成中断但保留了已获取上下文；IncompleteReason 为中断原因。
	Incomplete       bool           `json:"incomplete,omitempty"`
	IncompleteReason string         `json:"incomplete_reason,omitempty"`
	Diagnosis        *ChatDiagnosis `json:"diagnosis,omitempty"`
}

// buildChatUserPrompt 组装模型 user 消息：问题 + 上下文 + 只读观测 + 历史。
func buildChatUserPrompt(in ChatPromptInput) string {
	history := make([]map[string]string, 0, len(in.History))
	for _, item := range in.History {
		content := truncateRunes(strings.TrimSpace(item.Content), chatHistoryContentRunes)
		if content == "" {
			continue
		}
		history = append(history, map[string]string{"role": item.Role, "content": content})
	}
	now := in.Now
	if now <= 0 {
		// 兜底：任何调用方遗漏 Now 时仍给出服务端时间，避免模型自行推断时间基准。
		now = time.Now().Unix()
	}
	body := map[string]any{
		"question":     strings.TrimSpace(in.Question),
		"context":      in.Context,
		"time_range":   strings.TrimSpace(in.TimeRange),
		"now":          now,
		"alert":        in.Alert,
		"observations": in.Observations,
		"history":      history,
		"task":         chatPromptTask(in.ForceAnswer),
	}
	if len(in.Memory) > 0 {
		body["memory_cases"] = in.Memory
	}
	raw, err := json.Marshal(body)
	if err != nil {
		// 兜底：至少保留问题，避免模型收到空 prompt。
		return fmt.Sprintf(`{"question":%q}`, strings.TrimSpace(in.Question))
	}
	return string(raw)
}

// chatPromptTask 常规轮与强制收敛轮的任务指令。
func chatPromptTask(forceAnswer bool) string {
	if forceAnswer {
		return "取数预算已耗尽，禁止再输出 tool_requests；必须仅基于现有 observations 立即输出最终 answer JSON（含 summary）；数据不足时置 insufficient_context=true 并在 uncertainties 说明缺少什么，不得臆造未提供的数据。"
	}
	return "基于上下文与只读观测回答用户问题，并按固定 JSON 结构输出。"
}

// chatAnswerPayload 是模型输出的 JSON 结构；actions 由服务端生成，不接受模型覆盖。
// degraded / degraded_reason 仅在平台降级落库时写入，重放历史回答时按原值还原。
type chatAnswerPayload struct {
	Summary             string         `json:"summary"`
	Evidence            flexList       `json:"evidence"`
	Impact              flexList       `json:"impact"`
	Recommendations     flexList       `json:"recommendations"`
	RiskLevel           string         `json:"risk_level"`
	RequireConfirm      bool           `json:"require_confirm"`
	Rollback            flexText       `json:"rollback"`
	Uncertainties       flexList       `json:"uncertainties"`
	InsufficientContext bool           `json:"insufficient_context"`
	Degraded            bool           `json:"degraded"`
	DegradedReason      string         `json:"degraded_reason"`
	Incomplete          bool           `json:"incomplete"`
	IncompleteReason    string         `json:"incomplete_reason"`
	Diagnosis           *ChatDiagnosis `json:"diagnosis"`
}

// flexList 兼容模型把字符串数组漂移输出为对象数组等形态，逐元素转成可读字符串。
type flexList []string

func (l *flexList) UnmarshalJSON(raw []byte) error {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		*l = list
		return nil
	}
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		*l = nil
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
			continue
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			continue
		}
		out = append(out, string(encoded))
	}
	*l = out
	return nil
}

// flexText 兼容模型把文本字段输出为字符串或字符串数组。
type flexText string

func (t *flexText) UnmarshalJSON(raw []byte) error {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*t = flexText(strings.TrimSpace(single))
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		*t = flexText(strings.Join(sanitizeList(list), "；"))
		return nil
	}
	// 其他类型不阻断解析，按空文本处理。
	*t = ""
	return nil
}

// parseChatAnswer 解析并校验模型输出，失败表示需要降级。
func parseChatAnswer(content string) (ChatAnswer, error) {
	return parseChatAnswerWithDiagnosis(content, false)
}

func parseChatAnswerForReplay(content string) (ChatAnswer, error) {
	return parseChatAnswerWithDiagnosis(content, true)
}

func parseChatAnswerWithDiagnosis(content string, preserveDiagnosis bool) (ChatAnswer, error) {
	var payload chatAnswerPayload
	if err := json.Unmarshal([]byte(extractJSONObject(content)), &payload); err != nil {
		return ChatAnswer{}, fmt.Errorf("decode chat answer json: %w", err)
	}
	summary := strings.TrimSpace(payload.Summary)
	if summary == "" {
		return ChatAnswer{}, errors.New("chat answer summary is required")
	}
	answer := ChatAnswer{
		Summary:             truncateRunes(summary, 2000),
		Evidence:            sanitizeList(payload.Evidence),
		Impact:              sanitizeList(payload.Impact),
		Recommendations:     sanitizeList(payload.Recommendations),
		RiskLevel:           normalizeChatRisk(payload.RiskLevel),
		RequireConfirm:      payload.RequireConfirm,
		Actions:             []ChatAction{},
		Rollback:            truncateRunes(string(payload.Rollback), 1000),
		Uncertainties:       sanitizeList(payload.Uncertainties),
		InsufficientContext: payload.InsufficientContext,
		Degraded:            payload.Degraded,
		DegradedReason:      strings.TrimSpace(payload.DegradedReason),
		Incomplete:          payload.Incomplete,
		IncompleteReason:    strings.TrimSpace(payload.IncompleteReason),
		Diagnosis:           nil,
	}
	if preserveDiagnosis {
		answer.Diagnosis = payload.Diagnosis
	}
	return answer, nil
}

// chatResultStatus 由结构化回答还原对外 status（重放历史回答时使用）。
func chatResultStatus(answer ChatAnswer) string {
	if answer.Incomplete {
		return chatStatusIncomplete
	}
	if answer.Degraded {
		return chatStatusDegraded
	}
	if answer.InsufficientContext {
		return chatStatusInsufficientContext
	}
	return chatStatusOK
}

// degradedChatAnswer 是模型不可用/输出非法/流式中断时的保守降级结果。
func degradedChatAnswer(reason string, evidence []string) ChatAnswer {
	summary := "AI 助手暂未生成有效分析结论，已展示可用的只读上下文，请结合告警与监控人工判断。"
	uncertainties := make([]string, 0, 2)
	switch reason {
	case chatDegradedModelUnavailable:
		uncertainties = append(uncertainties, "尚未配置可用的平台直连模型（类型 b），请联系管理员在系统管理 / AI 配置中完成配置")
	case chatDegradedModelFailed:
		uncertainties = append(uncertainties, "模型调用超时或失败，请稍后重试；不要据此直接执行变更")
	case chatDegradedOutputInvalid:
		uncertainties = append(uncertainties, "模型输出无法通过平台结构校验，已丢弃并降级")
	case chatDegradedLoopExhausted:
		uncertainties = append(uncertainties, "模型在追加只读取数的预算内仍未给出结论，已按已获取上下文保守回答")
	case chatDegradedTurnBudgetExceeded:
		uncertainties = append(uncertainties, "本轮对话已超过总时间预算，已停止继续取数与模型调用；请缩小问题范围或稍后重试")
	case chatDegradedStreamInterrupted:
		summary = "AI 回答在生成过程中中断，已保留本轮已获取的只读证据；请缩小问题范围或稍后重试。"
		uncertainties = append(uncertainties, "模型流式输出中断，未获得完整结构化回答；已保留本轮已获取的只读证据")
	case chatDegradedOutputTruncated:
		summary = "AI 回答达到模型输出长度上限，已保留本轮已获取的只读证据；建议缩小问题范围后重试。"
		uncertainties = append(uncertainties, "模型输出达到长度上限，未获得完整结构化回答；已保留本轮已获取的只读证据，建议减少规则条数或缩短时间窗口")
	case chatDegradedRateLimited:
		uncertainties = append(uncertainties, "模型服务当前限流，重试后仍未恢复")
	default:
		uncertainties = append(uncertainties, "平台返回保守降级结果，未生成新的分析结论")
	}
	if len(evidence) == 0 {
		evidence = []string{"暂无可用只读观测数据，请结合告警详情与监控页面人工判断"}
	}
	return ChatAnswer{
		Summary:         summary,
		Evidence:        evidence,
		Impact:          []string{},
		Recommendations: []string{"稍后重试；涉及变更时通过自动化执行创建待确认任务，由人工确认后执行。"},
		RiskLevel:       "info",
		RequireConfirm:  false,
		Actions:         []ChatAction{},
		Rollback:        "无变更动作",
		Uncertainties:   uncertainties,
		Degraded:        true,
		DegradedReason:  reason,
	}
}

func normalizeChatRisk(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "medium", "high", "info":
		return strings.ToLower(strings.TrimSpace(raw))
	case "critical":
		// 平台执行侧的高风险统一按 high 呈现。
		return "high"
	default:
		return "info"
	}
}

func chatRiskRank(level string) int {
	switch level {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	default:
		return 0
	}
}

// sanitizeList 去除空白项、去重并限制条数与单条长度。
func sanitizeList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = truncateRunes(strings.TrimSpace(item), chatMaxListItemRunes)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if len(out) >= chatMaxListItems {
			break
		}
	}
	return out
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// extractJSONObject 从可能包含 markdown 代码块的模型输出中提取 JSON 对象。
func extractJSONObject(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return content
}
