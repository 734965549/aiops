# AI Agent Loop E2E (requires migration 0049 + running API in dev env + one active alert).
#
# Covers ops/ai-contract.md §10.16 with a scripted mock model:
#   MOCK_TOOL_LOOP     -> model requests alarm.analyze + metrics.query, loop executes read-only tools
#   MOCK_TOOL_DENY     -> model requests execution.tasks.propose, server denies without executing
#   MOCK_TOOL_DISCOVERY -> model requests accounts.list + resources.list (context discovery tools)
#   MOCK_TOOL_INFINITE -> model keeps requesting tools, loop stops after <=3 rounds with agent_loop_exhausted
#
# Audit chat_message.loop_rounds and tool_invoke/tool_denied entries are asserted.
#
# Usage:
#   .\scripts\e2e-ai-agent-loop.ps1
#   .\scripts\e2e-ai-agent-loop.ps1 -AlertId <alert_id> -MockPort 18082

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$AlertId = "",
    [int]$MockPort = 18081,
    [string]$MockBaseUrl = ""
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/e2e-ai-mock-provider.ps1"

function Step([string]$Message) {
    Write-Host "==> $Message"
}

function Find-ToolCall($toolCalls, [string]$Code) {
    return @($toolCalls | Where-Object { $_.tool_code -eq $Code })[0]
}

function Find-ChatAudit($audits, [string]$MessageId) {
    return @($audits.items | Where-Object {
        $_.action -eq "chat_message" -and [string]$_.payload.message_id -eq $MessageId
    })[0]
}

Write-Host "== AI Agent Loop E2E =="
Step "login as $Username"
$login = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

if (-not $AlertId) {
    Step "find latest active alert (required for loop tool execution)"
    try {
        $list = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path "/api/alerts?page=1&page_size=1&active_only=true" -Headers $auth
        if ($list.items -and $list.items.Count -gt 0) {
            $AlertId = [string]$list.items[0].id
        }
    } catch {
        Write-Host ("    warn: alert list unavailable: " + $_.Exception.Message)
    }
}
if (-not $AlertId) {
    Write-Host "SKIP: no active alert found; Agent Loop needs alert context to execute alarm.analyze."
    Write-Host "      Create/ingest an alert or pass -AlertId <alert_id> and re-run."
    exit 0
}

$mock = $null
$context = @{ page = "agent-loop-e2e"; page_title = "Agent Loop E2E"; alert_id = $AlertId; time_range = "30m" }

try {
    if ([string]::IsNullOrWhiteSpace($MockBaseUrl)) {
        Step "start scripted mock model on port $MockPort"
        $mock = Start-AiMockProvider -Port $MockPort
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -Port $MockPort | Out-Null
    } else {
        Step "use external mock model at $MockBaseUrl"
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -BaseUrl $MockBaseUrl | Out-Null
    }

    $session = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E Agent Loop 会话"
        context = $context
    }
    $sessionId = [string]$session.session_id

    Step "loop requests read-only tools (alarm.analyze + metrics.query)"
    $loop = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "请先核对告警快照与指标证据，再给出处置建议。MOCK_TOOL_LOOP"
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $loop -Context "agent loop"
    $alarmCall = Find-ToolCall $loop.tool_calls "alarm.analyze"
    if (-not $alarmCall -or -not $alarmCall.allowed -or $alarmCall.status -ne "ok") {
        throw ("expected allowed alarm.analyze observation, got: " + ($loop.tool_calls | ConvertTo-Json -Compress))
    }
    $metricCall = Find-ToolCall $loop.tool_calls "metrics.query"
    if (-not $metricCall) {
        throw "expected metrics.query observation from agent loop"
    }
    $loopAudits = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $sessionId + "&page=1&page_size=100") -Headers $auth
    $loopAudit = Find-ChatAudit $loopAudits $loop.message_id
    if (-not $loopAudit) { throw "missing loop chat_message audit" }
    if ([int]$loopAudit.payload.loop_rounds -lt 1) {
        throw ("expected loop_rounds>=1, got " + $loopAudit.payload.loop_rounds)
    }
    $toolInvokes = @($loopAudits.items | Where-Object {
        $_.action -eq "tool_invoke" -and $_.payload.tool_code -eq "alarm.analyze"
    })
    if ($toolInvokes.Count -lt 2) {
        # 首轮 gatherContext 与 loop 内追加取数各一条（loop 内的 message_id 为空）。
        throw ("expected >=2 alarm.analyze tool_invoke audits (initial + loop), got " + $toolInvokes.Count)
    }
    Write-Host ("    loop_rounds=" + $loopAudit.payload.loop_rounds + " alarm_analyze_invokes=" + $toolInvokes.Count)

    Step "execution tool request is denied inside the loop"
    $deny = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "尝试在循环里申请执行类工具。MOCK_TOOL_DENY"
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $deny -Context "agent loop deny"
    $denyCall = @($deny.tool_calls | Where-Object { $_.tool_code -eq "execution.tasks.propose" -and -not $_.allowed })[0]
    if (-not $denyCall -or $denyCall.reason -ne "tool is not allowed in agent loop") {
        throw ("expected execution.tasks.propose denied in loop, got: " + ($deny.tool_calls | ConvertTo-Json -Compress))
    }
    $denyAudits = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $sessionId + "&page=1&page_size=100") -Headers $auth
    $denyAudit = Find-ChatAudit $denyAudits $deny.message_id
    if (-not $denyAudit) { throw "missing deny chat_message audit" }
    if (@($denyAudit.payload.tool_denied) -notcontains "execution.tasks.propose") {
        throw ("audit tool_denied missing execution.tasks.propose: " + ($denyAudit.payload.tool_denied -join ","))
    }
    if ([int]$denyAudit.payload.loop_rounds -ne 1) {
        throw ("expected one denied round, got loop_rounds=" + $denyAudit.payload.loop_rounds)
    }
    Write-Host "    execution.tasks.propose denied (loop_rounds=1)"

    Step "context discovery tools (accounts.list + resources.list)"
    $discoveryAccountID = ""
    try {
        $accounts = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path "/api/integrations/accounts?page=1&page_size=1&enabled=true" -Headers $auth
        if ($accounts.items -and @($accounts.items).Count -gt 0) {
            $discoveryAccountID = [string]$accounts.items[0].account_id
        }
    } catch {
        Write-Host ("    warn: account list unavailable: " + $_.Exception.Message)
    }
    $discoveryMarker = "MOCK_TOOL_DISCOVERY"
    if ($discoveryAccountID) {
        $discoveryMarker = $discoveryMarker + ' {"account_id":"' + $discoveryAccountID + '"}'
    }
    $discovery = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "请先确认可用账号并列出 RDS 实例。" + $discoveryMarker
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $discovery -Context "context discovery"
    $accountsCall = Find-ToolCall $discovery.tool_calls "accounts.list"
    if (-not $accountsCall -or -not $accountsCall.allowed -or $accountsCall.status -ne "ok") {
        throw ("expected accounts.list observation, got: " + ($discovery.tool_calls | ConvertTo-Json -Compress))
    }
    $resourcesCall = Find-ToolCall $discovery.tool_calls "resources.list"
    if (-not $resourcesCall) {
        throw "expected resources.list observation from agent loop"
    }
    if ($discoveryAccountID) {
        # 有账号时资源发现应实际执行（列表为空也算 ok）；数据范围拒绝也视为链路正确。
        if (-not $resourcesCall.allowed -or ($resourcesCall.status -ne "ok" -and $resourcesCall.reason -ne "no access to integration account for evidence")) {
            throw ("unexpected resources.list observation: " + ($resourcesCall | ConvertTo-Json -Compress))
        }
    } elseif ([string]$resourcesCall.status -ne "skipped") {
        throw ("resources.list without account must be skipped, got " + $resourcesCall.status)
    }
    Write-Host ("    accounts.list=" + $accountsCall.status + " resources.list=" + $resourcesCall.status)

    Step "loop budget exhaustion degrades with agent_loop_exhausted"
    $exhaustSession = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E Agent Loop 预算会话"
        context = $context
    }
    $exhaust = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $exhaustSession.session_id + "/messages") -Headers $auth -Body @{
        content = "持续申请追加观测。MOCK_TOOL_INFINITE"
        context = $context
    }
    if ([string]$exhaust.status -ne "degraded") {
        throw ("expected degraded after loop budget exhaustion, got " + $exhaust.status)
    }
    if ([string]$exhaust.answer.degraded_reason -ne "agent_loop_exhausted") {
        throw ("expected agent_loop_exhausted, got " + $exhaust.answer.degraded_reason)
    }
    $exhaustAudits = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $exhaustSession.session_id + "&page=1&page_size=100") -Headers $auth
    $exhaustAudit = Find-ChatAudit $exhaustAudits $exhaust.message_id
    if (-not $exhaustAudit) { throw "missing exhaustion chat_message audit" }
    if ([int]$exhaustAudit.payload.loop_rounds -ne 3) {
        throw ("expected exactly 3 loop rounds, got " + $exhaustAudit.payload.loop_rounds)
    }
    if ([string]$exhaustAudit.payload.degraded_reason -ne "agent_loop_exhausted") {
        throw ("audit degraded_reason=" + $exhaustAudit.payload.degraded_reason)
    }
    Write-Host ("    degraded_reason=agent_loop_exhausted loop_rounds=" + $exhaustAudit.payload.loop_rounds)

    Write-Host ""
    Write-Host "PASS: AI Agent Loop E2E"
    Write-Host ("  session_id=" + $sessionId)
    Write-Host ("  alert_id=" + $AlertId)
}
finally {
    Unregister-AiMockProvider -ApiBase $ApiBase -Headers $auth
    Stop-AiMockProvider $mock
}
