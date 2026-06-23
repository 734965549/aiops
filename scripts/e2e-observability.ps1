# Observability fake provider E2E verification (requires PostgreSQL + running API + migrations through 0019).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-observability.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123"
)

$ErrorActionPreference = "Stop"

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = @{},
        $Body = $null
    )
    $uri = "$($ApiBase.TrimEnd('/'))$Path"
    $params = @{
        Uri = $uri
        Method = $Method
        Headers = $Headers
        UseBasicParsing = $true
    }
    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 12 -Compress)
    }
    $resp = Invoke-WebRequest @params
    $json = $resp.Content | ConvertFrom-Json
    if ($json.code -ne "OK") {
        throw "API $Method $Path failed: $($json.code) $($json.message)"
    }
    return $json.data
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

Write-Host "==> create fake huawei_cloud account"
$account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body @{
    name = "E2E Observability Fake"
    provider = "huawei_cloud"
    auth_type = "none"
    regions = @("cn-north-4")
    owner_team = "sre"
}
$accountId = $account.account_id
if (-not $accountId) {
    throw "create account response missing account_id"
}
Write-Host ("    account_id=" + $accountId)

Write-Host "==> connectivity check"
$check = Invoke-Api -Method POST -Path ("/api/integrations/accounts/" + $accountId + "/check") -Headers $auth
foreach ($cap in @("metrics", "logs", "traces", "topology")) {
    if ($check.capabilities -notcontains $cap) {
        throw "connectivity capabilities missing $cap"
    }
}

$to = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$from = $to - 3600

Write-Host "==> query metrics"
$metrics = Invoke-Api -Method POST -Path "/api/observability/metrics/query" -Headers $auth -Body @{
    account_id = $accountId
    provider = "huawei_cloud"
    region = "cn-north-4"
    metric = "cpu_util"
    from = $from
    to = $to
    period = 60
}
if (-not $metrics.evidence_id -or -not $metrics.series -or $metrics.series.Count -lt 1) {
    throw "metrics response missing evidence_id or series"
}
Write-Host ("    metrics_evidence_id=" + $metrics.evidence_id)

Write-Host "==> search logs"
$logs = Invoke-Api -Method POST -Path "/api/observability/logs/search" -Headers $auth -Body @{
    account_id = $accountId
    provider = "huawei_cloud"
    service = "payment-service"
    keyword = "timeout"
    from = $from
    to = $to
    limit = 10
}
if (-not $logs.evidence_id -or -not $logs.entries -or $logs.entries.Count -lt 1) {
    throw "logs response missing evidence_id or entries"
}
Write-Host ("    logs_evidence_id=" + $logs.evidence_id)

Write-Host "==> query traces"
$traces = Invoke-Api -Method POST -Path "/api/observability/traces/query" -Headers $auth -Body @{
    account_id = $accountId
    provider = "huawei_cloud"
    service = "payment-service"
    operation = "POST /pay"
    error_only = $true
    min_latency_ms = 500
    from = $from
    to = $to
    limit = 10
}
if (-not $traces.evidence_id -or -not $traces.spans -or $traces.spans.Count -lt 1) {
    throw "traces response missing evidence_id or spans"
}
if (-not $traces.spans[0].error) {
    throw "expected fake error span when error_only=true"
}
Write-Host ("    traces_evidence_id=" + $traces.evidence_id)

Write-Host "==> query topology"
$topology = Invoke-Api -Method GET -Path ("/api/observability/topology?account_id=" + $accountId + "&provider=huawei_cloud&application_id=app-e2e&from=" + $from + "&to=" + $to) -Headers $auth
if (-not $topology.evidence_id -or -not $topology.topology.nodes -or $topology.topology.nodes.Count -lt 1) {
    throw "topology response missing evidence_id or nodes"
}
Write-Host ("    topology_evidence_id=" + $topology.evidence_id)

Write-Host "==> cleanup account"
Invoke-Api -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null

Write-Host ""
Write-Host "PASS: Observability E2E verification completed"
