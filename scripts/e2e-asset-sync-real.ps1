# Cloud asset sync E2E with REAL huawei_cloud AK/SK account (manual reconciliation).
# 不进 CI：需要真实凭据与 CES 资源分组权限，由人工执行。
#
# 用途：
#   1. 用真实华为云 AK/SK 触发资产同步。
#   2. 打印每个 region 的 ces_total/discovered/upserted/failed_scopes/enriched/enrichment_failed 摘要。
#   3. 按 cloud_resource_type 分组统计平台入库数量，输出对账表。
#   4. 提示去 CES 控制台比对"全部资源/资源分组"总数。
#
# 前置条件：
#   - 后端已起（默认 http://127.0.0.1:8080），admin/admin123 可登录。
#   - 已运行 go run ./cmd/migrate。
#   - 拥有真实华为云子账号 AK/SK；默认 ces 模式仅需 CES 只读权限；
#     sync_mode=hybrid 还需 ECS/RDS/ELB/EVS/VPC/DCS/DMS 只读权限以做详情增强。
#   - 已在 CES 控制台创建或存在目标资源分组（默认候选名"全部资源"需用户预先创建；
#     CES 官方 API 只返回用户创建的资源分组，不存在"总览全量"隐式口径；
#     默认候选名未命中将直接失败，不回退到最大资源组，因此生产推荐显式指定 resource_group_id）。
#
# 用法示例（推荐环境变量，避免 AK/SK 进入 shell 历史与进程参数）：
#   $env:HUAWEI_ACCESS_KEY="AKXXX"; $env:HUAWEI_SECRET_KEY="SKXXX"
#   $env:HUAWEI_PROJECT_ID="0xxx"; $env:HUAWEI_REGIONS="cn-south-1"
#   .\scripts\e2e-asset-sync-real.ps1
#   .\scripts\e2e-asset-sync-real.ps1 -ExtraConfig '{"sync_mode":"hybrid"}'   # 覆盖 ExtraConfig
#   未设置环境变量时，脚本会交互读取 AccessKey/ProjectId/Regions/SecretKey。
#
# 安全：
#   - AK/SK 优先从环境变量读取，未设置时交互输入（SecretKey 用 SecureString）。
#   - AK/SK 不写日志；脚本结束默认删除账号。注意：删除账号为软删除（保留账号行用于审计），
#     后端会在删除时一并硬删除 integration_credential_ref 中的凭据密文。
#   - 建议使用只读权限子账号，避免使用主账号凭据。

param(
    [string]$AccessKey = $(if ($env:HUAWEI_ACCESS_KEY) { $env:HUAWEI_ACCESS_KEY } else { "" }),
    [string]$SecretKey = $(if ($env:HUAWEI_SECRET_KEY) { $env:HUAWEI_SECRET_KEY } else { "" }),
    [string]$ProjectId = $(if ($env:HUAWEI_PROJECT_ID) { $env:HUAWEI_PROJECT_ID } else { "" }),
    [string]$Regions = $(if ($env:HUAWEI_REGIONS) { $env:HUAWEI_REGIONS } else { "" }),
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$ExtraConfig = "",
    [switch]$KeepAccount
)

$ErrorActionPreference = "Stop"

# 优先从环境变量读取凭据；未提供时交互输入，避免 AK/SK 进入 shell 历史与进程参数。
if (-not $AccessKey) { $AccessKey = Read-Host "AccessKey" }
if (-not $ProjectId) { $ProjectId = Read-Host "ProjectId" }
if (-not $Regions) { $Regions = Read-Host "Regions (逗号分隔，如 cn-south-1,cn-north-4)" }
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

function Wait-SyncBatch {
    param(
        [hashtable]$Headers,
        [string]$BatchId,
        [int]$IntervalMs = 1000,
        [int]$TimeoutMs = 600000
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

$regionList = $Regions -split '[,，\s]+' | Where-Object { $_ } | ForEach-Object { $_.Trim() }
if ($regionList.Count -eq 0) {
    throw "Regions 解析为空，请用逗号分隔，如 cn-south-1,cn-north-4"
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
        throw "ExtraConfig 不是合法 JSON: $_"
    }
}
$account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body $accountBody
$accountId = $account.account_id
if (-not $accountId) {
    throw "create account response missing account_id"
}
Write-Host ("    account_id=" + $accountId)

# 失败时兜底清理。
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
        Write-Host ("==> KeepAccount=true, account_id=" + $accountId + " 保留，请手动清理")
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
        $batch = Wait-SyncBatch -Headers $auth -BatchId $batch.batch_id
    }
    Write-Host ("    batch_id=" + $batch.batch_id)
    Write-Host ("    status=" + $batch.status)
    Write-Host ("    message=" + $batch.message)
    Write-Host ("    created=" + $batch.created_count + " updated=" + $batch.updated_count + " failed=" + $batch.failed_count + " stale=" + $batch.stale_count)

    if ($batch.status -ne "success" -and $batch.status -ne "partial") {
        throw "expected sync status success or partial, got $($batch.status)"
    }
    if ($batch.status -eq "partial") {
        Write-Warning "sync status=partial; 基础同步结果以批次 message 为准，继续对账"
    }

    Write-Host ""
    Write-Host "==> synced resources by cloud_resource_type"
    $script:appId = $batch.application_id
    if (-not $script:appId) {
        throw "sync response missing application_id"
    }
    # 后端 page_size 上限 100，翻页拉取全部资源后再按 source 聚合，避免总量被截断。
    # 统计口径排除 sync_status=stale 的历史资源，只对账当前云端仍存在的资源：
    # 第二次同步会把云端已消失的资源标记为 stale，若不排除会把历史资源算进平台总数，
    # 可能错误宣告与 CES 数量一致。同时按 region 分组，便于与 CES 各区域资源数对账。
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
    Write-Host ("    application resources total = " + $appTotal + " (含 manual)")
    Write-Host ("    total active cloud_sync resources = " + $cloudItems.Count + " (已排除 stale=" + $staleCount + ")")
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
    Write-Host "    请前往 CES 控制台 -> 云监控 -> 资源分组 -> 全部资源，"
    Write-Host "    按 region 比对 CES 各区域资源总数与上方 total active cloud_sync resources（已排除 stale）。"
    Write-Host "    若 sync_mode=hybrid，还需核对 EVS/VPC/DCS/DMS 增强是否命中（检查批次 message 的 enriched 计数）。"
    Write-Host "    匹配键（ProviderRef）与 CES dim value 不一致会导致 enriched=0，需校正 mapper 字段。"

    Write-Host ""
    Write-Host "PASS: Real account reconciliation data collected"
} catch {
    Write-Error ("对账失败: " + $_)
    throw
} finally {
    Cleanup-Account
}
