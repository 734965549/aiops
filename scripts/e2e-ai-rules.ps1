# AI rule inspection E2E (requires migrations through 0055 + running API in dev env).
#
# Covers ops/ai-contract.md §10.12.1 with a scripted mock model and a fake huawei_cloud account:
#   alert.rules.list params (namespace/limit) -> rule evidence + threshold fields
#   metrics.query free params (30-day daily window) -> metric evidence
#   tool_calls / references / tool_invoke evidence_id / loop_rounds audit
#
# The mock model requests both tools in round 1 and returns an answer in round 2 only
# when the rule and metric observations reached the prompt.
#
# Usage:
#   .\scripts\e2e-ai-rules.ps1
#   .\scripts\e2e-ai-rules.ps1 -MockPort 18082

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
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

Write-Host "== AI rule inspection E2E =="
Step "login as $Username"
$login = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

$mock = $null
$accountId = ""
try {
    Step "create fake huawei_cloud account"
    $account = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body @{
        name = "E2E Rules Inspection Fake"
        provider = "huawei_cloud"
        auth_type = "none"
        regions = @("cn-north-4")
        owner_team = "sre"
    }
    $accountId = [string]$account.account_id
    if (-not $accountId) { throw "create account response missing account_id" }
    Write-Host ("    account_id=" + $accountId)

    if ([string]::IsNullOrWhiteSpace($MockBaseUrl)) {
        Step "start scripted mock model on port $MockPort"
        $mock = Start-AiMockProvider -Port $MockPort
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -Port $MockPort | Out-Null
    } else {
        Step "use external mock model at $MockBaseUrl"
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -BaseUrl $MockBaseUrl | Out-Null
    }

    $to = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $from = $to - 30 * 24 * 3600
    $params = @{
        account_id = $accountId
        region = "cn-north-4"
        namespace = "SYS.RDS"
        metric = "cpu_util"
        dimensions = @{ instance_id = "i-1" }
        from = $from
        to = $to
        period = 86400
    }
    $question = "请体检 SYS.RDS 命名空间的告警规则阈值是否合理。MOCK_TOOL_RULES " + ($params | ConvertTo-Json -Depth 8 -Compress)
    $context = @{ page = "rules-e2e"; page_title = "规则体检 E2E"; time_range = "30d" }

    Step "create chat session and request rule inspection"
    $session = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E 规则体检会话"
        context = $context
    }
    $sessionId = [string]$session.session_id
    $sent = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = $question
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $sent -Context "rule inspection"
    if ($sent.answer.summary -notmatch "规则体检" -or $sent.answer.summary -notmatch "指标证据") {
        throw ("mock answer did not observe rule + metric evidence: " + $sent.answer.summary)
    }
    Write-Host ("    answer=" + $sent.answer.summary)

    Step "tool calls include rules + free-param metrics with evidence"
    $rulesCall = Find-ToolCall $sent.tool_calls "alert.rules.list"
    if (-not $rulesCall -or -not $rulesCall.allowed -or $rulesCall.status -ne "ok") {
        throw ("expected allowed alert.rules.list ok, got: " + ($sent.tool_calls | ConvertTo-Json -Compress))
    }
    $metricsCall = Find-ToolCall $sent.tool_calls "metrics.query"
    if (-not $metricsCall -or -not $metricsCall.allowed -or $metricsCall.status -ne "ok") {
        throw ("expected allowed metrics.query ok (30d daily window), got: " + ($sent.tool_calls | ConvertTo-Json -Compress))
    }
    $evidenceRefs = @($sent.references | Where-Object { $_.type -eq "observability_evidence" })
    if ($evidenceRefs.Count -lt 2) {
        throw ("expected >=2 observability evidence references, got: " + ($sent.references | ConvertTo-Json -Compress))
    }
    Write-Host ("    evidence_refs=" + $evidenceRefs.Count)

    Step "audit carries loop_rounds and evidence ids"
    $audits = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $sessionId + "&page=1&page_size=100") -Headers $auth
    $chatAudit = @($audits.items | Where-Object {
        $_.action -eq "chat_message" -and [string]$_.payload.message_id -eq [string]$sent.message_id
    })[0]
    if (-not $chatAudit) { throw "missing chat_message audit" }
    if ([int]$chatAudit.payload.loop_rounds -lt 1) {
        throw ("expected loop_rounds>=1, got " + $chatAudit.payload.loop_rounds)
    }
    $toolCodes = @($chatAudit.payload.tool_codes)
    foreach ($code in @("alert.rules.list", "metrics.query")) {
        if ($toolCodes -notcontains $code) { throw ("chat_message audit missing tool code " + $code) }
    }
    foreach ($code in @("alert.rules.list", "metrics.query")) {
        $row = @($audits.items | Where-Object {
            $_.action -eq "tool_invoke" -and $_.payload.tool_code -eq $code -and $_.payload.evidence_id
        })[0]
        if (-not $row) { throw ("tool_invoke audit missing evidence_id for " + $code) }
    }

    Write-Host ""
    Write-Host "PASS: AI rule inspection E2E"
    Write-Host ("  session_id=" + $sessionId)
    Write-Host ("  account_id=" + $accountId)
    Write-Host ("  loop_rounds=" + $chatAudit.payload.loop_rounds)
}
finally {
    Unregister-AiMockProvider -ApiBase $ApiBase -Headers $auth
    Stop-AiMockProvider $mock
    if ($accountId) {
        try {
            Invoke-AiApi -ApiBase $ApiBase -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null
            Write-Host ("    fake account removed: " + $accountId)
        } catch {
            Write-Host ("    warn: remove fake account failed: " + $_.Exception.Message)
        }
    }
}
