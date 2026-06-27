# Cloud asset sync E2E (fake huawei_cloud account, no real cloud credentials).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-asset-sync.ps1

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

# 触发同步后立即返回 running 批次，需轮询到终态再断言。
function Wait-SyncBatch {
    param(
        [hashtable]$Headers,
        [string]$BatchId,
        [int]$IntervalMs = 1000,
        [int]$TimeoutMs = 60000
    )
    $deadline = [System.Diagnostics.Stopwatch]::StartNew()
    while ($true) {
        $b = Invoke-Api -Method GET -Path "/api/assets/sync/batches/$BatchId" -Headers $Headers
        if ($b.status -ne "running") {
            return $b
        }
        if ($deadline.ElapsedMilliseconds -gt $TimeoutMs) {
            throw "sync batch $BatchId still running after ${TimeoutMs}ms"
        }
        Start-Sleep -Milliseconds $IntervalMs
    }
}

function Remove-CloudSyncApplication {
    param(
        [hashtable]$Headers,
        [string]$ApplicationId
    )
    if (-not $ApplicationId) { return }
    $pageSize = 100
    $page = 1
    $fetched = 0
    $total = 0
    $resourceIds = @()
    do {
        $resources = Invoke-Api -Method GET -Path ("/api/assets/applications/" + $ApplicationId + "/resources?page=$page&page_size=$pageSize") -Headers $Headers
        $total = [int]$resources.total
        $items = @($resources.items)
        $resourceIds += @($items | Where-Object { $_.source -eq "cloud_sync" } | ForEach-Object { $_.id })
        $fetched += $items.Count
        $page++
    } while ($fetched -lt $total -and $items.Count -gt 0)
    foreach ($resourceId in @($resourceIds | Select-Object -Unique)) {
        Invoke-Api -Method DELETE -Path ("/api/assets/resources/" + $resourceId) -Headers $Headers | Out-Null
    }
    Invoke-Api -Method DELETE -Path ("/api/assets/applications/" + $ApplicationId) -Headers $Headers | Out-Null
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

Write-Host "==> create fake huawei_cloud account for asset sync"
$account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body @{
    name = "E2E Asset Sync Fake"
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

Write-Host "==> trigger cloud asset sync"
$batch = Invoke-Api -Method POST -Path "/api/assets/sync" -Headers $auth -Body @{
    account_id = $accountId
}
if (-not $batch.batch_id) {
    throw "sync response missing batch_id"
}
if ($batch.status -ne "running") {
    throw "expected sync status running immediately after trigger, got $($batch.status)"
}
# 异步同步：轮询到终态再断言。
$batch = Wait-SyncBatch -Headers $auth -BatchId $batch.batch_id
if ($batch.status -ne "success") {
    throw "expected sync status success, got $($batch.status) message=$($batch.message)"
}
if (($batch.created_count + $batch.updated_count) -lt 1) {
    throw "expected at least one synced resource"
}
Write-Host ("    batch_id=" + $batch.batch_id + " created=" + $batch.created_count + " updated=" + $batch.updated_count)

Write-Host "==> list sync batches"
$batches = Invoke-Api -Method GET -Path ("/api/assets/sync/batches?account_id=" + $accountId + "&page=1&page_size=10") -Headers $auth
if (-not $batches.items -or $batches.items.Count -lt 1) {
    throw "expected sync batch history"
}

Write-Host "==> verify synced resources in asset registry"
$appId = $batch.application_id
if (-not $appId) {
    throw "sync response missing application_id"
}
$resources = Invoke-Api -Method GET -Path ("/api/assets/applications/" + $appId + "/resources") -Headers $auth
$cloudItems = @($resources.items | Where-Object { $_.source -eq "cloud_sync" })
if ($cloudItems.Count -lt 1) {
    throw "expected cloud_sync resources in application registry"
}
$sample = $cloudItems[0]
if (-not $sample.cloud_resource_id) {
    throw "expected cloud_resource_id on synced resource"
}
Write-Host ("    cloud_resource_id=" + $sample.cloud_resource_id + " type=" + $sample.cloud_resource_type)

Write-Host "==> second sync keeps stable fake inventory active"
$batch2 = Invoke-Api -Method POST -Path "/api/assets/sync" -Headers $auth -Body @{
    account_id = $accountId
}
if ($batch2.status -ne "running") {
    throw "expected second sync status running immediately after trigger, got $($batch2.status)"
}
$batch2 = Wait-SyncBatch -Headers $auth -BatchId $batch2.batch_id
if ($batch2.status -ne "success") {
    throw "second sync unexpected status $($batch2.status)"
}
if ($batch2.stale_count -ne 0) {
    throw "stable fake inventory should not mark stale resources, stale_count=$($batch2.stale_count)"
}
$resources2 = Invoke-Api -Method GET -Path ("/api/assets/applications/" + $appId + "/resources") -Headers $auth
$staleItems = @($resources2.items | Where-Object { $_.sync_status -eq "stale" })
if ($staleItems.Count -ne 0) {
    throw "stable fake inventory should not contain stale resources"
}

Write-Host "==> cleanup synced cloud resources, application and account"
Remove-CloudSyncApplication -Headers $auth -ApplicationId $appId
Invoke-Api -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null

Write-Host ""
Write-Host "PASS: Asset cloud sync E2E verification completed"
