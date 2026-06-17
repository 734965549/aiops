# Execution task E2E (requires PostgreSQL migration 0011 + running API + alert from e2e-alert).
# Usage:
#   .\scripts\e2e-alert.ps1
#   .\scripts\e2e-execution.ps1 -AlertId <alert_id from e2e>

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$AlertId = ""
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
        $params.Body = ($Body | ConvertTo-Json -Depth 10 -Compress)
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

if (-not $AlertId) {
    Write-Host "==> find latest processing alert"
    $list = Invoke-Api -Method GET -Path "/api/alerts?page=1&page_size=5&status=processing" -Headers $auth
    if (-not $list.items -or $list.items.Count -eq 0) {
        throw "no processing alert found; run scripts/e2e-alert.ps1 first or pass -AlertId"
    }
    $AlertId = $list.items[0].id
}
Write-Host ("    alert_id=" + $AlertId)

Write-Host "==> create execution task from alert"
$created = Invoke-Api -Method POST -Path "/api/executions/tasks" -Headers $auth -Body @{
    source_type = "alert"
    source_id = $AlertId
    operation_type = "restart"
    rollback_plan = @{ description = "E2E rollback note" }
}
Write-Host ("    task_id=" + $created.task_id + " status=" + $created.status)
if ($created.status -ne "pending_confirm") {
    throw "expected pending_confirm for restart, got $($created.status)"
}

Write-Host "==> confirm task"
$confirmed = Invoke-Api -Method POST -Path ("/api/executions/tasks/" + $created.task_id + "/confirm") -Headers $auth -Body @{
    confirm = $true
    confirm_text = "CONFIRM"
}
if ($confirmed.status -ne "pending_execute") {
    throw "expected pending_execute after confirm, got $($confirmed.status)"
}

Write-Host "==> execute task"
$detail = Invoke-Api -Method POST -Path ("/api/executions/tasks/" + $created.task_id + "/execute") -Headers $auth -Body @{}
if ($detail.task.status -ne "success") {
    throw "expected success, got $($detail.task.status)"
}
Write-Host ("    result=" + $detail.task.result_summary)

Write-Host "==> verify alert timeline"
$alertDetail = Invoke-Api -Method GET -Path ("/api/alerts/" + $AlertId) -Headers $auth
$types = @($alertDetail.events | ForEach-Object { $_.event_type })
foreach ($t in @("execution_created", "execution_started", "execution_finished")) {
    if ($types -notcontains $t) {
        throw "alert timeline missing event: $t"
    }
}

Write-Host "==> E2E execution passed"
