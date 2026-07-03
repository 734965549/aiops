# Cloud asset sync E2E with REAL huawei_cloud AK/SK account (manual reconciliation).
# Not for CI: requires real credentials and CES resource group permissions.
#
# Purpose:
#   1. Trigger asset sync with real Huawei Cloud AK/SK.
#   2. Print ces_total/discovered/upserted/failed_scopes/enriched/enrichment_failed summaries by region.
#   3. Count stored assets by cloud_resource_type and print a reconciliation table.
#   4. Remind operators to compare totals in the CES console.
#
# Prerequisites:
#   - Backend is running (default http://127.0.0.1:8080), admin/admin123 can login.
#   - Migrations have been applied.
#   - A real Huawei Cloud sub-account AK/SK is available. CES mode needs CES read-only permissions;
#     sync_mode=hybrid also needs ECS/RDS/ELB/EVS/VPC/DCS/DMS read-only permissions for enrichment.
#   - The target CES resource group already exists. Specify resource_group_id explicitly for production reconciliation.
#
# Usage examples (environment variables are recommended to avoid AK/SK in shell history and process args):
#   $env:HUAWEI_ACCESS_KEY="AKXXX"; $env:HUAWEI_SECRET_KEY="SKXXX"
#   $env:HUAWEI_PROJECT_ID="0xxx"; $env:HUAWEI_REGIONS="cn-south-1"
#   .\scripts\e2e-asset-sync-real.ps1
#   .\scripts\e2e-asset-sync-real.ps1 -ExtraConfig '{"sync_mode":"hybrid"}'   # override ExtraConfig
#   -SyncTimeoutMs defaults to 35 minutes, intentionally greater than the backend 30-minute hard timeout,
#   so cleanup does not race with a still-running background sync.
#   If env vars are missing, the script prompts for AccessKey/ProjectId/Regions/SecretKey.
#
# Security:
#   - AK/SK are read from env vars first; SecretKey uses SecureString when prompted.
#   - AK/SK are not logged. The script deletes the account by default.
#   - Use a read-only sub-account instead of a root account.

param(
    [string]$AccessKey = $(if ($env:HUAWEI_ACCESS_KEY) { $env:HUAWEI_ACCESS_KEY } else { "" }),
    [string]$SecretKey = $(if ($env:HUAWEI_SECRET_KEY) { $env:HUAWEI_SECRET_KEY } else { "" }),
    [string]$ProjectId = $(if ($env:HUAWEI_PROJECT_ID) { $env:HUAWEI_PROJECT_ID } else { "" }),
    [string]$Regions = $(if ($env:HUAWEI_REGIONS) { $env:HUAWEI_REGIONS } else { "" }),
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$ExtraConfig = "",
    [int]$SyncTimeoutMs = 2100000,
    [switch]$KeepAccount
)

$ErrorActionPreference = "Stop"

# Prefer credentials from environment variables; prompt when missing to avoid AK/SK in shell history and process args.
if (-not $AccessKey) { $AccessKey = Read-Host "AccessKey" }
if (-not $ProjectId) { $ProjectId = Read-Host "ProjectId" }
if (-not $Regions) { $Regions = Read-Host "Regions (comma-separated, e.g. cn-south-1,cn-north-4)" }
if (-not $SecretKey) {
    $ss = Read-Host "SecretKey" -AsSecureString
    $SecretKey = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($ss))
}

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = @{},
        $Body = $null
    )
    $uri = "$($ApiBase.TrimEnd('/'))$Path"
    $params = @{
        Uri     = $uri
        Method  = $Method
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

function WaitSyncBatch {
    param(
        [hashtable]$Headers,
        [string]$BatchId,
        [int]$IntervalMs = 1000,
        [int]$TimeoutMs = 2100000
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

$regionList = $Regions -split '[,\s]+' | Where-Object { $_ } | ForEach-Object { $_.Trim() }
if ($regionList.Count -eq 0) {
    throw "Regions is empty; use comma-separated values such as cn-south-1,cn-north-4"
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

Write-Host "==> create REAL huawei_cloud account (auth_type=ak_sk)"
$accountBody = @{
    name       = "E2E Real Reconcile"
    provider   = "huawei_cloud"
    auth_type  = "ak_sk"
    regions    = $regionList
    project_id = $ProjectId
    owner_team = "sre"
    credential = @{
        access_key = $AccessKey
        secret_key = $SecretKey
    }
}
if ($ExtraConfig -ne "") {
    try {
        $parsed = $ExtraConfig | ConvertFrom-Json
        $accountBody.extra_config = $parsed
    } catch {
        throw "ExtraConfig is not valid JSON: $_"
    }
}
$account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body $accountBody
$accountId = $account.account_id
if (-not $accountId) {
    throw "create account response missing account_id"
}
Write-Host ("    account_id=" + $accountId)

# Best-effort cleanup on failure.
$appId = ""
function Cleanup-Account {
    if (-not $KeepAccount -and $accountId) {
        Write-Host "==> cleanup synced cloud resources, application and account"
        try {
            Remove-CloudSyncApplication -Headers $auth -ApplicationId $appId
        } catch {
            Write-Warning ("cleanup cloud resources/application failed: " + $_)
        }
        try {
            Invoke-Api -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null
        } catch {
            Write-Warning ("cleanup account failed: " + $_)
        }
    } else {
        Write-Host ("==> KeepAccount=true, account_id=" + $accountId + " kept; please clean up manually")
    }
}

try {
    Write-Host "==> trigger cloud asset sync (real CES discovery)"
    $batch = Invoke-Api -Method POST -Path "/api/assets/sync" -Headers $auth -Body @{
        account_id = $accountId
    }
    if (-not $batch.batch_id) {
        throw "sync response missing batch_id"
    }
    if ($batch.status -eq "running") {
        $batch = WaitSyncBatch -Headers $auth -BatchId $batch.batch_id -TimeoutMs $SyncTimeoutMs
    }
    if (-not $batch.application_id) {
        throw "sync response missing application_id"
    }
    $script:appId = $batch.application_id
    if (-not $batch.summary) {
        throw "sync response missing summary"
    }
    Write-Host ("    batch_id=" + $batch.batch_id)
    Write-Host ("    application_id=" + $script:appId)
    Write-Host ("    status=" + $batch.status)
    Write-Host ("    summary=" + ($batch.summary | ConvertTo-Json -Depth 3 -Compress))
    Write-Host ("    created=" + $batch.created_count + " updated=" + $batch.updated_count + " failed=" + $batch.failed_count + " stale=" + $batch.stale_count)

    if ($batch.status -ne "success" -and $batch.status -ne "partial") {
        throw "expected sync status success or partial, got $($batch.status) summary=$($batch.summary)"
    }
    if ($batch.status -eq "partial") {
        Write-Warning "sync status=partial; continue reconciliation with batch message context"
    }

    Write-Host ""
    Write-Host "==> synced resources by cloud_resource_type"
    # Backend page_size max is 100. Fetch all resources before grouping.
    # Exclude sync_status=stale so historical resources do not affect current reconciliation.
    $pageSize = 100
    $page = 1
    $cloudItems = @()
    $staleCount = 0
    $fetched = 0
    $appTotal = 0
    do {
        $resources = Invoke-Api -Method GET -Path ("/api/assets/applications/" + $script:appId + "/resources?page=$page&page_size=$pageSize") -Headers $auth
        $appTotal = [int]$resources.total
        $items = @($resources.items)
        $staleCount += @($items | Where-Object { $_.source -eq "cloud_sync" -and $_.sync_status -eq "stale" }).Count
        $cloudItems += @($items | Where-Object { $_.source -eq "cloud_sync" -and $_.sync_status -ne "stale" })
        $fetched += $items.Count
        $page++
    } while ($fetched -lt $appTotal -and $items.Count -eq $pageSize)
    if ($cloudItems.Count -lt 1) {
        throw "expected active cloud_sync resources in application registry"
    }
    $byRegion = @{}
    $byRegionType = @{}
    foreach ($item in $cloudItems) {
        $r = $item.region
        if (-not $r) { $r = "<unknown>" }
        if ($byRegion.ContainsKey($r)) {
            $byRegion[$r] = $byRegion[$r] + 1
        } else {
            $byRegion[$r] = 1
        }
        $t = $item.cloud_resource_type
        if (-not $t) { $t = "<unknown>" }
        $key = "$r|$t"
        if ($byRegionType.ContainsKey($key)) {
            $byRegionType[$key] = $byRegionType[$key] + 1
        } else {
            $byRegionType[$key] = 1
        }
    }
    Write-Host ("    application resources total = " + $appTotal + " (including manual)")
    Write-Host ("    total active cloud_sync resources = " + $cloudItems.Count + " (excluded stale=" + $staleCount + ")")
    Write-Host "    ---- region ----      ---- count ----"
    foreach ($r in ($byRegion.Keys | Sort-Object)) {
        Write-Host ("    {0,-20} {1}" -f $r, $byRegion[$r])
    }
    Write-Host "    ---- region/type ----             ---- count ----"
    foreach ($k in ($byRegionType.Keys | Sort-Object)) {
        Write-Host ("    {0,-34} {1}" -f $k, $byRegionType[$k])
    }

    Write-Host ""
    Write-Host "==> reconciliation reminder"
    Write-Host "    Compare the active cloud_sync resource count above with the CES console resource group total."
    Write-Host "    For sync_mode=hybrid, also check summary.enriched_count / summary.enrichment_failed_types."
    Write-Host "    If ProviderRef and CES dimension values differ, enriched may be zero and mapper fields need adjustment."

    Write-Host ""
    Write-Host "PASS: Real account reconciliation data collected"
} catch {
    Write-Error ("reconciliation failed: " + $_)
    throw
} finally {
    Cleanup-Account
}
