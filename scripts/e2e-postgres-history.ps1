# RDS for PostgreSQL history evidence E2E (requires migration 0061 and a long-transaction alert).
# Uses the alert's existing integration account; no database credential is printed or persisted by this script.
# Required Huawei IAM actions:
#   rds:instance:getHistorySession / rds:instance:getWaitEvent / rds:instance:getTopSql
param(
    [Parameter(Mandatory = $true)][string]$AlertId,
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [int]$MockPort = 18084,
    [string]$MockBaseUrl = ""
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/e2e-ai-mock-provider.ps1"

function Step([string]$Message) { Write-Host "==> $Message" }

Write-Host "== RDS PostgreSQL history evidence E2E =="
Step "login as $Username"
$login = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }
$mock = $null
try {
    if ([string]::IsNullOrWhiteSpace($MockBaseUrl)) {
        Step "start scripted mock model on port $MockPort"
        $mock = Start-AiMockProvider -Port $MockPort
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -Port $MockPort | Out-Null
    } else {
        Step "use external mock model at $MockBaseUrl"
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -BaseUrl $MockBaseUrl | Out-Null
    }

    $context = @{ page = "postgres-history-e2e"; page_title = "PostgreSQL 历史会话验收"; alert_id = $AlertId; time_range = "1h" }
    $session = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E PostgreSQL 历史会话"
        context = $context
    }
    $sessionId = [string]$session.session_id

    Step "collect history through the alert's existing cloud account"
    $sent = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "请根据 RDS 历史会话证据分析长事务原因，不要猜测。"
        context = $context
    }
    $call = $sent.tool_calls | Where-Object { $_.tool_code -eq 'postgres.activity.query' } | Select-Object -First 1
    if (-not $call) {
        throw 'postgres.activity.query was not executed; verify alert labels and RDS instance id'
    }
    if ([string]$call.status -ne "ok") {
        throw ("history collection failed: status=" + $call.status + " reason=" + $call.reason + "; check rds:instance:getHistorySession / getWaitEvent / getTopSql and instance history-session support")
    }
    $diagnosis = $sent.answer.diagnosis
    if (-not $diagnosis -or -not $diagnosis.root_cause) {
        throw "structured diagnosis is missing"
    }
    if ([string]$diagnosis.root_cause.code -eq "idle_in_transaction") {
        if ([string]$diagnosis.root_cause.status -ne "confirmed") {
            throw ("idle transaction category must be confirmed by direct session evidence, got " + $diagnosis.root_cause.status)
        }
        if (-not ([string]$diagnosis.root_cause.statement).Contains("仍未定")) {
            throw "confirmed category must not claim the application-level reason"
        }
        if (@($diagnosis.missing_evidence) -notcontains "application_logs") {
            throw "idle transaction diagnosis must request application logs for the unfinished transaction reason"
        }
    }

    Write-Host ""
    Write-Host "PASS: RDS PostgreSQL history evidence E2E"
    Write-Host ("  tool_summary=" + $call.summary)
    Write-Host ("  root_status=" + $diagnosis.root_cause.status + " root_code=" + $diagnosis.root_cause.code)
    Write-Host ("  evidence_ids=" + ($diagnosis.root_cause.evidence_ids -join ","))
    Write-Host ("  missing_evidence=" + ($diagnosis.missing_evidence -join ","))
    foreach ($fact in @($diagnosis.facts | Select-Object -First 6)) {
        Write-Host ("  fact=" + $fact.statement + " [" + $fact.evidence_id + $fact.pointer + "]")
    }
}
finally {
    Unregister-AiMockProvider -ApiBase $ApiBase -Headers $auth
    Stop-AiMockProvider $mock
}
