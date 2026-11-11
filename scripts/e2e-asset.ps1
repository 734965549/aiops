# Asset module end-to-end verification (requires PostgreSQL + running API).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   .\scripts\e2e-asset.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$AppName = "",
    [string]$SourceId = "e2e-asset-am",
    [string]$WebhookSecret = "e2e-asset-webhook-secret"
)

$ErrorActionPreference = "Stop"

$RunId = [guid]::NewGuid().ToString("N").Substring(0, 8)
if ([string]::IsNullOrWhiteSpace($AppName)) {
    $AppName = "registered-$RunId"
}
$ServiceLabel = "payment-e2e-$RunId"
$RulePattern = "$ServiceLabel*"
$RulePriority = [int][DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
$PodName = "payment-e2e-$RunId"
$ResourceName = "payment-pod-$RunId"
$Fingerprint = "e2e-asset-fp-$RunId"

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
    try {
        $resp = Invoke-WebRequest @params
    } catch {
        $message = $_.Exception.Message
        $response = $_.Exception.Response
        if ($null -ne $response) {
            try {
                $stream = $response.GetResponseStream()
                if ($null -ne $stream) {
                    $reader = New-Object System.IO.StreamReader($stream)
                    $bodyText = $reader.ReadToEnd()
                    if (-not [string]::IsNullOrWhiteSpace($bodyText)) {
                        try {
                            $errorJson = $bodyText | ConvertFrom-Json
                            $message = "$($errorJson.code) $($errorJson.message)"
                        } catch {
                            $message = $bodyText
                        }
                    }
                }
            } catch {
                # Keep the original WebException message when the body cannot be read.
            }
        }
        throw "API $Method $Path failed: $message"
    }
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
$token = $login.access_token
$auth = @{ Authorization = "Bearer $token" }

Write-Host "==> create application $AppName"
$app = Invoke-Api -Method POST -Path "/api/assets/applications" -Headers $auth -Body @{
    name = $AppName
    environment = "prod"
    namespace = "payment"
    description = "E2E asset application"
}
$appId = $app.id
Write-Host ("    application_id=" + $appId)

Write-Host "==> create resource with pod $PodName"
$res = Invoke-Api -Method POST -Path "/api/assets/resources" -Headers $auth -Body @{
    application_id = $appId
    name = $ResourceName
    resource_type = "pod"
    namespace = "payment"
    pod = $PodName
}
$resId = $res.id
Write-Host ("    resource_id=" + $resId)

Write-Host ("==> create match rule service=" + $RulePattern)
$rule = Invoke-Api -Method POST -Path "/api/assets/match-rules" -Headers $auth -Body @{
    name = "E2E payment rule"
    enabled = $true
    priority = $RulePriority
    target_type = "resource"
    source_type = "all"
    label_key = "service"
    label_value_pattern = $RulePattern
    application_id = $appId
    resource_id = $resId
}
Write-Host ("    rule_id=" + $rule.id)

$listRes = Invoke-Api -Method GET -Path ("/api/assets/applications/" + $appId + "/resources") -Headers $auth
if ($listRes.items.Count -lt 1) {
    throw "resource list should contain at least one item"
}

Write-Host "==> seed extra resources for filter/sort assertions"
$extraIds = @()
foreach ($suffix in @("zz", "aa")) {
    $extra = Invoke-Api -Method POST -Path "/api/assets/resources" -Headers $auth -Body @{
        application_id = $appId
        name = "sort-$suffix-$RunId"
        resource_type = "pod"
        namespace = "payment"
    }
    $extraIds += $extra.id
}

$resourcesPath = "/api/assets/applications/" + $appId + "/resources?page=1&page_size=100"

Write-Host "==> assert keyword/source/region filters"
$filtered = Invoke-Api -Method GET -Path ($resourcesPath + "&keyword=" + $RunId) -Headers $auth
if ($filtered.total -ne 3) {
    throw ("keyword filter should match 3 resources, got " + $filtered.total)
}
$wildcard = Invoke-Api -Method GET -Path ($resourcesPath + "&keyword=%25") -Headers $auth
if ($wildcard.total -ne 0) {
    throw ("literal %% must not act as LIKE wildcard, got " + $wildcard.total)
}
$manual = Invoke-Api -Method GET -Path ($resourcesPath + "&source=manual") -Headers $auth
if ($manual.total -ne 3) {
    throw ("source=manual should match 3 resources, got " + $manual.total)
}
$cloud = Invoke-Api -Method GET -Path ($resourcesPath + "&source=cloud_sync") -Headers $auth
if ($cloud.total -ne 0) {
    throw ("source=cloud_sync should match 0 resources, got " + $cloud.total)
}
$region = Invoke-Api -Method GET -Path ($resourcesPath + "&region=cn-north-4") -Headers $auth
if ($region.total -ne 0) {
    throw ("region filter on manual resources should match 0, got " + $region.total)
}

Write-Host "==> assert server-side sorting"
$asc = Invoke-Api -Method GET -Path ($resourcesPath + "&sort_by=name&sort_order=asc") -Headers $auth
$ascNames = @($asc.items | ForEach-Object { $_.name })
if ($asc.total -ne 3 -or $ascNames[0] -ne $ResourceName -or $ascNames[1] -ne ("sort-aa-" + $RunId) -or $ascNames[2] -ne ("sort-zz-" + $RunId)) {
    throw ("unexpected ascending order: " + ($ascNames -join ","))
}
$desc = Invoke-Api -Method GET -Path ($resourcesPath + "&sort_by=name&sort_order=desc") -Headers $auth
$descNames = @($desc.items | ForEach-Object { $_.name })
if ($descNames[0] -ne ("sort-zz-" + $RunId) -or $descNames[2] -ne $ResourceName) {
    throw ("unexpected descending order: " + ($descNames -join ","))
}
$invalidSort = Invoke-Api -Method GET -Path ($resourcesPath + "&sort_by=id%3Bdrop&sort_order=asc") -Headers $auth
if ($invalidSort.total -ne 3 -or $invalidSort.items[0].name -ne $ResourceName) {
    throw "invalid sort_by must fall back to created_at ASC"
}

Write-Host "==> assert resource filter facets"
$facets = Invoke-Api -Method GET -Path ("/api/assets/applications/" + $appId + "/resource-facets") -Headers $auth
if ($null -eq $facets.cloud_resource_types -or $null -eq $facets.regions) {
    throw "resource-facets must return cloud_resource_types and regions arrays"
}
if ($facets.cloud_resource_types.Count -ne 0 -or $facets.regions.Count -ne 0) {
    throw ("manual-only application should have empty facets, got " + ($facets | ConvertTo-Json -Compress))
}

Write-Host ("==> create alert source " + $SourceId)
try {
    Invoke-Api -Method POST -Path "/api/alerts/sources" -Headers $auth -Body @{
        id = $SourceId
        name = "E2E Asset Alertmanager"
        type = "prometheus_alertmanager"
        enabled = $true
        secret = $WebhookSecret
        environment = "prod"
    } | Out-Null
} catch {
    Write-Host "    source may already exist, continue"
}

$fingerprint = $Fingerprint
$firingPayload = @{
    status = "firing"
    alerts = @(
        @{
            status = "firing"
            fingerprint = $fingerprint
            labels = @{
                alertname = "HighMemory"
                severity = "critical"
                service = $ServiceLabel
                env = "prod"
                namespace = "payment"
                pod = $PodName
            }
            annotations = @{
                summary = "Memory > 90%"
                description = "E2E asset match test"
            }
            startsAt = "2026-06-13T00:00:00Z"
        }
    )
}

Write-Host "==> ingest alert with matching labels"
$ingest = Invoke-Api -Method POST -Path ("/api/alerts/ingest/alertmanager/" + $SourceId) -Headers @{
    "X-AIOPS-Webhook-Token" = $WebhookSecret
} -Body $firingPayload
Write-Host ("    created=" + $ingest.created + " updated=" + $ingest.updated)

$list = Invoke-Api -Method GET -Path ("/api/alerts?source_id=" + $SourceId + "&active_only=true") -Headers $auth
if ($list.total -lt 1) { throw "alert list should contain at least one item" }
$alertId = $list.items[0].id
Write-Host ("    alert_id=" + $alertId)

$detail = Invoke-Api -Method GET -Path ("/api/alerts/" + $alertId) -Headers $auth
if ($detail.alert.application_id -ne $appId) {
    throw ("expected application_id=" + $appId + ", got " + $detail.alert.application_id)
}
if ($detail.alert.resource_id -ne $resId) {
    throw ("expected resource_id=" + $resId + ", got " + $detail.alert.resource_id)
}
Write-Host ("    matched application_id=" + $detail.alert.application_id + " resource_id=" + $detail.alert.resource_id)
if (-not $detail.alert.application_name) {
    throw "expected application_name to be populated from labels"
}

Write-Host "==> update application description"
$updatedApp = Invoke-Api -Method PUT -Path ("/api/assets/applications/" + $appId) -Headers $auth -Body @{
    name = $AppName
    environment = "prod"
    namespace = "payment"
    description = "E2E updated description"
}
if ($updatedApp.description -ne "E2E updated description") {
    throw "application update failed"
}

Write-Host "==> delete application with resources should fail"
$deleteBlocked = $false
try {
    Invoke-Api -Method DELETE -Path ("/api/assets/applications/" + $appId) -Headers $auth | Out-Null
} catch {
    if ($_.Exception.Message -match "FAILED_PRECONDITION|412") {
        $deleteBlocked = $true
    } else {
        throw
    }
}
if (-not $deleteBlocked) {
    throw "expected delete application to fail when resources exist"
}
Write-Host "    blocked as expected"

Write-Host "==> delete resource with match rule should fail"
$deleteResBlocked = $false
try {
    Invoke-Api -Method DELETE -Path ("/api/assets/resources/" + $resId) -Headers $auth | Out-Null
} catch {
    if ($_.Exception.Message -match "FAILED_PRECONDITION|412") {
        $deleteResBlocked = $true
    } else {
        throw
    }
}
if (-not $deleteResBlocked) {
    throw "expected delete resource to fail when match rules exist"
}
Write-Host "    blocked as expected"

Write-Host "==> delete match rule then resources and application"
Invoke-Api -Method POST -Path ("/api/alerts/" + $alertId + "/close") -Headers $auth -Body @{
    resolution = "E2E asset cleanup"
} | Out-Null
Invoke-Api -Method DELETE -Path ("/api/assets/match-rules/" + $rule.id) -Headers $auth | Out-Null
foreach ($extraId in $extraIds) {
    Invoke-Api -Method DELETE -Path ("/api/assets/resources/" + $extraId) -Headers $auth | Out-Null
}
Invoke-Api -Method DELETE -Path ("/api/assets/resources/" + $resId) -Headers $auth | Out-Null
Invoke-Api -Method DELETE -Path ("/api/assets/applications/" + $appId) -Headers $auth | Out-Null
Write-Host "    cleanup completed"

Write-Host ""
Write-Host "PASS: Asset E2E verification completed"
