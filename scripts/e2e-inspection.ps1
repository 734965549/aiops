# Inspection E2E verification (requires PostgreSQL + running API + migrations through 0020).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-inspection.ps1

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

Write-Host "==> create fake huawei_cloud account for inspection scope"
$account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body @{
    name = "E2E Inspection Fake"
    provider = "huawei_cloud"
    auth_type = "none"
    regions = @("cn-north-4")
    owner_team = "sre"
}
$accountId = $account.account_id
Write-Host ("    account_id=" + $accountId)

Write-Host "==> create inspection policy"
$policy = Invoke-Api -Method POST -Path "/api/inspections/policies" -Headers $auth -Body @{
    name = "E2E Core App Inspection"
    enabled = $true
    schedule = "*/15 * * * *"
    scope = @{
        environment = "prod"
        account_id = $accountId
        provider = "huawei_cloud"
        application_ids = @("payment-service")
        resource_types = @("service", "host")
    }
    checks = @(
        "metrics.cpu",
        "metrics.memory",
        "traces.error_rate",
        "logs.error_burst"
    )
    agent_profile = "sre_default"
}
$policyId = $policy.policy_id
if (-not $policyId) {
    throw "create policy response missing policy_id"
}
Write-Host ("    policy_id=" + $policyId)

Write-Host "==> trigger manual inspection run"
$run = Invoke-Api -Method POST -Path ("/api/inspections/policies/" + $policyId + "/runs") -Headers $auth
$runId = $run.run_id
if (-not $runId) {
    throw "trigger run response missing run_id"
}
Write-Host ("    run_id=" + $runId + " status=" + $run.status)

Write-Host "==> get run detail"
$runDetail = Invoke-Api -Method GET -Path ("/api/inspections/runs/" + $runId) -Headers $auth
if ($runDetail.status -notin @("success", "partial")) {
    throw "expected run status success or partial, got $($runDetail.status)"
}
if (-not $runDetail.timeline -or $runDetail.timeline.Count -lt 2) {
    throw "run timeline missing events"
}
Write-Host ("    run_status=" + $runDetail.status + " summary=" + $runDetail.summary)

Write-Host "==> list findings"
$findings = Invoke-Api -Method GET -Path ("/api/inspections/findings?run_id=" + $runId + "&page_size=20") -Headers $auth
if (-not $findings.items -or $findings.items.Count -lt 1) {
    throw "expected at least one finding"
}
$f0 = $findings.items[0]
if (-not $f0.finding_id -or -not $f0.risk_level -or -not $f0.evidence_refs -or $f0.evidence_refs.Count -lt 1) {
    throw "finding missing required fields (risk, evidence_refs)"
}
if (-not $f0.recommendations -or $f0.recommendations.Count -lt 1) {
    throw "finding missing recommendations"
}
Write-Host ("    findings=" + $findings.items.Count + " first_risk=" + $f0.risk_level + " evidence=" + $f0.evidence_refs[0])

Write-Host "==> cleanup policy and account"
Invoke-Api -Method DELETE -Path ("/api/inspections/policies/" + $policyId) -Headers $auth | Out-Null
Invoke-Api -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null

Write-Host ""
Write-Host "PASS: Inspection E2E verification completed"
