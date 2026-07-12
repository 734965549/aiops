# Cloud asset sync E2E with REAL huawei_cloud AK/SK account (automated reconciliation).
# Not for CI: requires real credentials and CES resource group permissions.
#
# Purpose:
#   1. Trigger asset sync with real Huawei Cloud AK/SK.
#   2. Print ces_total/discovered/upserted/failed_scopes/enriched/enrichment_failed summaries by region.
#   3. Count stored assets by cloud_resource_type and print a reconciliation table.
#   4. Verify §9.5 three count identities; assert active cloud_sync count == summary.persisted_count only on success batches; reconcile gap vs ces_total.
#   5. Assert application_id uses 0032 new format cloud-<prefix17>-<sha1_12hex>.
#   6. Assert batch status consistent with failure indicators (all scope failed -> failed).
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
    if ($script:appId -notmatch '^cloud-.{1,17}-[a-f0-9]{12}$') {
        throw "application_id does not match expected new format cloud-<prefix17>-<sha1_12hex>: $script:appId"
    }
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

    # Assert batch status is consistent with failure indicators.
    # Backend finalize contract (internal/asset/application/sync_service.go):
    #   failed   (non-cancel): failed_count>0 AND created+updated==0 AND no discovered scope
    #   partial : failed_count>0 OR max_resources_reached OR product_names_empty
    #   success : otherwise
    if ($batch.status -eq "success" -and $batch.failed_count -gt 0) {
        throw "status=success but failed_count=$($batch.failed_count) > 0"
    }
    if ($batch.status -eq "failed" -and ($batch.created_count + $batch.updated_count) -gt 0) {
        throw "status=failed but created+updated=$($batch.created_count + $batch.updated_count) > 0"
    }
    if ($batch.status -eq "partial") {
        # partial 合法原因对齐 sync_service.go finalize (line 455-458)：
        #   batch.failed_count>0 || max_resources_reached || product_names_empty
        #   || enrichment_failed_types || enrichment_failed_count>0
        #   || invalid_resource_count>0 || conversion_failed_types || query_failed_types
        # batch.failed_count 已含 failed_scopes 与 persist 失败（见 sync_service.go line 917/824/850），
        # 故不再单独遍历 scopes[].failed_scopes；顶层聚合字段等价于 finalize 的 anySummaryBool。
        $hasValidPartialReason = ($batch.failed_count -gt 0)
        if ($batch.summary) {
            if ($batch.summary.max_resources_reached) { $hasValidPartialReason = $true }
            if ($batch.summary.product_names_empty) { $hasValidPartialReason = $true }
            if (([int]$batch.summary.enrichment_failed_count) -gt 0) { $hasValidPartialReason = $true }
            if ($batch.summary.enrichment_failed_types -and $batch.summary.enrichment_failed_types.Count -gt 0) { $hasValidPartialReason = $true }
            if (([int]$batch.summary.invalid_resource_count) -gt 0) { $hasValidPartialReason = $true }
            if ($batch.summary.conversion_failed_types -and $batch.summary.conversion_failed_types.Count -gt 0) { $hasValidPartialReason = $true }
            if ($batch.summary.query_failed_types -and $batch.summary.query_failed_types.Count -gt 0) { $hasValidPartialReason = $true }
        }
        if (-not $hasValidPartialReason) {
            throw "status=partial but no valid partial reason (failed_count=0 and none of max_resources_reached/product_names_empty/enrichment_failed/invalid_resource/conversion_failed/query_failed)"
        }
    }
    # "全部 scope 失败"判定对齐后端 failed 条件：每个 scope 都有 failed_scopes 且无任何
    # successful_types/discovered_count（即无发现成功），同时无资源入库时，才期望 failed。
    # CES 单 region 聚合多个 namespace，某 scope 可能既有 failed_scopes（部分 namespace 失败）
    # 又有 successful_types（其他 namespace 成功），此时正确状态是 partial 而非 failed。
    if ($batch.summary -and $batch.summary.scopes -and $batch.summary.scopes.Count -gt 0) {
        $allScopesEntirelyFailed = $true
        foreach ($scope in $batch.summary.scopes) {
            $hasSuccess = ($scope.successful_types -and $scope.successful_types.Count -gt 0) -or ($scope.discovered_count -gt 0)
            if (-not $scope.failed_scopes -or $scope.failed_scopes.Count -eq 0 -or $hasSuccess) {
                $allScopesEntirelyFailed = $false
                break
            }
        }
        if ($allScopesEntirelyFailed -and ($batch.created_count + $batch.updated_count) -eq 0 -and $batch.status -ne "failed") {
            throw "all scopes entirely failed (failed_scopes present, no successful_types, nothing upserted) but status=$($batch.status); expected failed"
        }
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
    $summary = $batch.summary
    if (-not $summary) {
        throw "sync response missing summary"
    }
    # §9.5 三条对账恒等式，见 ops/huawei-ces-sync-contract.md §9.5：
    #   raw_fetched_count        = mapped_count + invalid_resource_count
    #   mapped_count             = unique_discovered_count + duplicate_count
    #   unique_discovered_count  = persisted_count + persist_failed_count
    $rawFetchedCount = [int]$summary.raw_fetched_count
    $mappedCount = [int]$summary.mapped_count
    $invalidResourceCount = [int]$summary.invalid_resource_count
    $uniqueDiscoveredCount = [int]$summary.unique_discovered_count
    $duplicateCount = [int]$summary.duplicate_count
    $persistedCount = [int]$summary.persisted_count
    $persistFailedCount = [int]$summary.persist_failed_count
    if ($rawFetchedCount -ne ($mappedCount + $invalidResourceCount)) {
        throw "§9.5 identity 1 broken: raw_fetched_count($rawFetchedCount) != mapped_count($mappedCount) + invalid_resource_count($invalidResourceCount)"
    }
    if ($mappedCount -ne ($uniqueDiscoveredCount + $duplicateCount)) {
        throw "§9.5 identity 2 broken: mapped_count($mappedCount) != unique_discovered_count($uniqueDiscoveredCount) + duplicate_count($duplicateCount)"
    }
    if ($uniqueDiscoveredCount -ne ($persistedCount + $persistFailedCount)) {
        throw "§9.5 identity 3 broken: unique_discovered_count($uniqueDiscoveredCount) != persisted_count($persistedCount) + persist_failed_count($persistFailedCount)"
    }
    # failed_count 是批次顶层字段(batch.failed_count)，不在 summary 内；
    # persist 失败同时计入 batch.failed_count 与 summary.persist_failed_count。
    $cesTotal = $summary.ces_total
    if ($null -ne $cesTotal -and $cesTotal -gt 0 -and $uniqueDiscoveredCount -gt $cesTotal) {
        throw "summary.unique_discovered_count $uniqueDiscoveredCount > ces_total $cesTotal"
    }
    if ($null -ne $cesTotal -and $cesTotal -gt 0 -and $uniqueDiscoveredCount -lt $cesTotal) {
        Write-Warning "unique_discovered_count $uniqueDiscoveredCount is less than ces_total $cesTotal; investigate mapper/gap"
    }
    # active 数量强一致断言仅在完整 success 批次做：FinalizeSuccess 会 stale 旧 active 资产并
    # 激活当批资源，故 active cloud_sync count == persisted_count（success 时 persist_failed_count=0）。
    # partial 批次保留旧 active 资产，不能做全局相等断言。
    if ($batch.status -eq "success") {
        if ($cloudItems.Count -ne $persistedCount) {
            throw "success batch active cloud_sync count($($cloudItems.Count)) != summary.persisted_count($persistedCount); expected strong consistency after FinalizeSuccess"
        }
    } else {
        Write-Warning "status=$($batch.status): skip active-count strong-consistency assertion (old active assets may be retained)"
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
