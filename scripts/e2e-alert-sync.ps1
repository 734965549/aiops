# CES alert pull E2E (fake huawei_cloud account, no real cloud credentials).
# Requires API running with a non-prod env so auth_type=none pull accounts are allowed.
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-alert-sync.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$SourceId = ("e2e-ces-" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds())
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

function Invoke-ApiExpectStatus {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = @{},
        $Body = $null,
        [int]$Status
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
    $thrown = $false
    try {
        Invoke-WebRequest @params | Out-Null
    } catch {
        $thrown = $true
        $resp = $_.Exception.Response
        if (-not $resp) { throw }
        $actual = [int]$resp.StatusCode
        if ($actual -ne $Status) {
            throw "API $Method $Path expected HTTP $Status, got $actual"
        }
    }
    if (-not $thrown) {
        throw "API $Method $Path expected HTTP $Status, got 2xx"
    }
}

function Get-Alerts {
    param([hashtable]$Headers, [string]$Source, [int]$PageSize = 100)
    return Invoke-Api -Method GET -Path ("/api/alerts?source_id=" + $Source + "&page=1&page_size=$PageSize") -Headers $Headers
}

$accountId = $null
$sourceCreated = $false

try {
    Write-Host "==> login as $Username"
    $login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
        username = $Username
        password = $Password
    }
    $auth = @{ Authorization = "Bearer $($login.access_token)" }

    Write-Host "==> create fake huawei_cloud account for CES alarm pull"
    $account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body @{
        name = "E2E CES Alert Pull Fake"
        provider = "huawei_cloud"
        auth_type = "none"
        regions = @("cn-north-4")
        project_id = "proj-e2e-alert-sync"
        owner_team = "sre"
    }
    $accountId = $account.account_id
    if (-not $accountId) {
        throw "create account response missing account_id"
    }
    if (@($account.capabilities) -notcontains "alerts") {
        throw "fake huawei_cloud account is missing alerts capability"
    }
    Write-Host ("    account_id=" + $accountId)

    Write-Host ("==> create pull source " + $SourceId)
    $source = Invoke-Api -Method POST -Path "/api/alerts/sources" -Headers $auth -Body @{
        id = $SourceId
        name = "E2E CES Pull"
        type = "huawei_ces"
        delivery_mode = "pull"
        account_id = $accountId
        enabled = $true
        environment = "prod"
        pull_config = @{
            pull_interval_seconds = 0
            region_filter = @("cn-north-4")
        }
    }
    $sourceCreated = $true
    if ($source.delivery_mode -ne "pull") {
        throw "expected delivery_mode=pull, got $($source.delivery_mode)"
    }
    if ($source.account_id -ne $accountId) {
        throw "expected account_id=$accountId, got $($source.account_id)"
    }
    if ([int]$source.pull_config.pull_interval_seconds -ne 0) {
        throw "expected pull_interval_seconds=0, got $($source.pull_config.pull_interval_seconds)"
    }
    if ($source.last_sync_at) {
        throw "new pull source should not have last_sync_at"
    }
    Write-Host "    pull source created"

    Write-Host "==> webhook source must reject sync"
    $webhookId = $SourceId + "-webhook"
    Invoke-Api -Method POST -Path "/api/alerts/sources" -Headers $auth -Body @{
        id = $webhookId
        name = "E2E CES Webhook"
        type = "huawei_ces"
        enabled = $true
        secret = "e2e-ces-webhook-secret"
    } | Out-Null
    try {
        Invoke-ApiExpectStatus -Method POST -Path ("/api/alerts/sources/" + $webhookId + "/sync") -Headers $auth -Body @{} -Status 400
    } finally {
        try {
            Invoke-Api -Method DELETE -Path ("/api/alerts/sources/" + $webhookId) -Headers $auth | Out-Null
        } catch {
            Write-Host "    cleanup webhook source skipped"
        }
    }

    Write-Host "==> sync round 1: two firing records"
    $r1 = Invoke-Api -Method POST -Path ("/api/alerts/sources/" + $SourceId + "/sync") -Headers $auth -Body @{}
    if ($r1.status -ne "success") {
        throw "round1 expected status success, got $($r1.status)"
    }
    if ($r1.created -ne 2 -or $r1.updated -ne 0 -or $r1.recovered -ne 0) {
        throw "round1 unexpected counts: created=$($r1.created) updated=$($r1.updated) recovered=$($r1.recovered)"
    }
    if ($r1.failed_records -ne 0 -or @($r1.failed_scopes).Count -ne 0) {
        throw "round1 unexpected failures: records=$($r1.failed_records) scopes=$(@($r1.failed_scopes).Count)"
    }
    Write-Host ("    created=" + $r1.created + " duration_ms=" + $r1.duration_ms)

    $alerts1 = Get-Alerts -Headers $auth -Source $SourceId
    if ([int]$alerts1.total -ne 2) {
        throw "round1 expected 2 alerts, got $($alerts1.total)"
    }
    foreach ($item in @($alerts1.items)) {
        if ($item.status -ne "new") {
            throw "round1 alert $($item.external_id) expected status new, got $($item.status)"
        }
        if ($item.severity -ne "p1") {
            throw "round1 alert $($item.external_id) expected severity p1, got $($item.severity)"
        }
        if ($item.source -ne "huawei_ces" -or $item.source_id -ne $SourceId) {
            throw "round1 alert $($item.external_id) has unexpected source attribution"
        }
        if ($item.occurrence_count -ne 1) {
            throw "round1 alert $($item.external_id) expected occurrence_count=1, got $($item.occurrence_count)"
        }
    }

    $sourceAfter1 = Invoke-Api -Method GET -Path ("/api/alerts/sources/" + $SourceId) -Headers $auth
    if ($sourceAfter1.last_sync_status -ne "success" -or -not $sourceAfter1.last_sync_at) {
        throw "source last_sync_* not persisted after round1"
    }

    Write-Host "==> sync round 2: one record recovers, one keeps firing"
    $r2 = Invoke-Api -Method POST -Path ("/api/alerts/sources/" + $SourceId + "/sync") -Headers $auth -Body @{}
    if ($r2.status -ne "success") {
        throw "round2 expected status success, got $($r2.status)"
    }
    if ($r2.created -ne 0 -or $r2.recovered -ne 1 -or $r2.updated -ne 1) {
        throw "round2 unexpected counts: created=$($r2.created) updated=$($r2.updated) recovered=$($r2.recovered) ignored=$($r2.ignored)"
    }

    $alerts2 = Get-Alerts -Headers $auth -Source $SourceId
    if ([int]$alerts2.total -ne 2) {
        throw "round2 expected 2 alerts, got $($alerts2.total)"
    }
    $byExternal = @{}
    foreach ($item in @($alerts2.items)) {
        $byExternal[$item.external_id] = $item
    }
    $recordA = @($byExternal.Keys | Where-Object { $_ -like "*-a" })[0]
    $recordB = @($byExternal.Keys | Where-Object { $_ -like "*-b" })[0]
    if (-not $recordA -or -not $recordB) {
        throw "round2 missing expected external ids"
    }
    if ($byExternal[$recordA].status -ne "recovered") {
        throw "round2 record A expected recovered, got $($byExternal[$recordA].status)"
    }
    if ($byExternal[$recordB].status -ne "new") {
        throw "round2 record B expected new, got $($byExternal[$recordB].status)"
    }
    if ($byExternal[$recordB].occurrence_count -lt 2) {
        throw "round2 record B expected occurrence_count>=2, got $($byExternal[$recordB].occurrence_count)"
    }

    Write-Host "==> sync round 3: both records recovered"
    $r3 = Invoke-Api -Method POST -Path ("/api/alerts/sources/" + $SourceId + "/sync") -Headers $auth -Body @{}
    if ($r3.status -ne "success") {
        throw "round3 expected status success, got $($r3.status)"
    }
    if ($r3.created -ne 0 -or $r3.recovered -ne 1 -or $r3.ignored -ne 1) {
        throw "round3 unexpected counts: created=$($r3.created) recovered=$($r3.recovered) ignored=$($r3.ignored)"
    }
    $alerts3 = Get-Alerts -Headers $auth -Source $SourceId
    foreach ($item in @($alerts3.items)) {
        if ($item.status -ne "recovered") {
            throw "round3 alert $($item.external_id) expected recovered, got $($item.status)"
        }
    }

    Write-Host "==> verify alert_source_sync audit trail"
    $audits = Invoke-Api -Method GET -Path ("/api/audits?resource_type=alert_source&resource_id=" + $SourceId + "&action=alert_source_sync&page=1&page_size=10") -Headers $auth
    if ([int]$audits.total -lt 3) {
        throw "expected at least 3 alert_source_sync audit rows, got $($audits.total)"
    }
    $latest = @($audits.items)[0]
    if (-not $latest.payload -or $latest.payload.status -eq $null) {
        throw "latest sync audit missing payload status"
    }

    Write-Host ""
    Write-Host "PASS: CES alert pull E2E verification completed"
} finally {
    if ($sourceCreated -and $auth) {
        Write-Host "==> cleanup pull source"
        try {
            Invoke-Api -Method DELETE -Path ("/api/alerts/sources/" + $SourceId) -Headers $auth | Out-Null
        } catch {
            Write-Host "    cleanup source skipped"
        }
    }
    if ($accountId -and $auth) {
        Write-Host "==> cleanup integration account"
        try {
            Invoke-Api -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null
        } catch {
            Write-Host "    cleanup account skipped"
        }
    }
}
