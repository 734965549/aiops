# Alert module end-to-end verification (requires PostgreSQL + running API).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   .\scripts\e2e-alert.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$SourceId = "e2e-am",
    [string]$WebhookSecret = "e2e-webhook-secret-123"
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
$token = $login.access_token
$auth = @{ Authorization = "Bearer $token" }
$userId = $login.user.id
Write-Host ("    user_id=" + $userId)

Write-Host ("==> create alert source " + $SourceId)
try {
    Invoke-Api -Method POST -Path "/api/alerts/sources" -Headers $auth -Body @{
        id = $SourceId
        name = "E2E Alertmanager"
        type = "prometheus_alertmanager"
        enabled = $true
        secret = $WebhookSecret
        environment = "prod"
    } | Out-Null
} catch {
    Write-Host "    source may already exist, continue"
}

$firingPayload = @{
    status = "firing"
    alerts = @(
        @{
            status = "firing"
            fingerprint = "e2e-fp-001"
            labels = @{
                alertname = "HighCPU"
                severity = "critical"
                instance = "node-1"
            }
            annotations = @{
                summary = "CPU > 90%"
                description = "E2E test alert"
            }
            startsAt = "2026-06-13T00:00:00Z"
        }
    )
}

Write-Host "==> send firing payload"
$ingest1 = Invoke-Api -Method POST -Path ("/api/alerts/ingest/alertmanager/" + $SourceId) -Headers @{
    "X-AIOPS-Webhook-Token" = $WebhookSecret
} -Body $firingPayload
Write-Host ("    created=" + $ingest1.created + " updated=" + $ingest1.updated)

Write-Host "==> send duplicate firing"
$ingest2 = Invoke-Api -Method POST -Path ("/api/alerts/ingest/alertmanager/" + $SourceId) -Headers @{
    "X-AIOPS-Webhook-Token" = $WebhookSecret
} -Body $firingPayload
Write-Host ("    created=" + $ingest2.created + " updated=" + $ingest2.updated)
if ($ingest2.updated -lt 1) { throw "duplicate firing should update existing alert" }

$list1 = Invoke-Api -Method GET -Path ("/api/alerts?source_id=" + $SourceId + "&active_only=true") -Headers $auth
if ($list1.total -lt 1) { throw "alert list should contain at least one item" }
$alertId = $list1.items[0].id
$count1 = $list1.items[0].occurrence_count
Write-Host ("    alert_id=" + $alertId + " occurrence_count=" + $count1)

Write-Host "==> send resolved payload"
$resolvedPayload = @{
    status = "resolved"
    alerts = @(
        @{
            status = "resolved"
            fingerprint = "e2e-fp-001"
            labels = @{ alertname = "HighCPU" }
            endsAt = "2026-06-13T00:05:00Z"
        }
    )
}
$ingest3 = Invoke-Api -Method POST -Path ("/api/alerts/ingest/alertmanager/" + $SourceId) -Headers @{
    "X-AIOPS-Webhook-Token" = $WebhookSecret
} -Body $resolvedPayload
Write-Host ("    recovered=" + $ingest3.recovered)

$detail1 = Invoke-Api -Method GET -Path ("/api/alerts/" + $alertId) -Headers $auth
if ($detail1.alert.status -ne "recovered") {
    throw ("expected recovered status, got " + $detail1.alert.status)
}
Write-Host ("    status=recovered events=" + $detail1.events.Count)

Write-Host "==> close alert"
Invoke-Api -Method POST -Path ("/api/alerts/" + $alertId + "/close") -Headers $auth -Body @{
    resolution = "E2E close"
} | Out-Null

Write-Host "==> new firing after close"
$ingest4 = Invoke-Api -Method POST -Path ("/api/alerts/ingest/alertmanager/" + $SourceId) -Headers @{
    "X-AIOPS-Webhook-Token" = $WebhookSecret
} -Body $firingPayload
Write-Host ("    created=" + $ingest4.created)

$list2 = Invoke-Api -Method GET -Path ("/api/alerts?source_id=" + $SourceId + "&active_only=true") -Headers $auth
$alert2 = $list2.items | Where-Object { $_.status -eq "new" } | Select-Object -First 1
if (-not $alert2) { throw "expected a new alert with status=new" }
$alert2Id = $alert2.id
Write-Host ("    new_alert_id=" + $alert2Id)

Write-Host "==> state flow: acknowledge -> start-processing -> recover"
Invoke-Api -Method POST -Path ("/api/alerts/" + $alert2Id + "/acknowledge") -Headers $auth -Body @{} | Out-Null
Invoke-Api -Method POST -Path ("/api/alerts/" + $alert2Id + "/start-processing") -Headers $auth -Body @{} | Out-Null
Invoke-Api -Method POST -Path ("/api/alerts/" + $alert2Id + "/recover") -Headers $auth -Body @{ message = "manual recover" } | Out-Null

Write-Host "==> AI analysis entry"
Invoke-Api -Method POST -Path ("/api/alerts/" + $alert2Id + "/ai-analysis") -Headers $auth -Body @{
    time_range = "30m"
    include_logs = $true
} | Out-Null
$ai = Invoke-Api -Method POST -Path "/api/ai/analyze-alert" -Headers $auth -Body @{
    alert_id = $alert2Id
    time_range = "30m"
    include_logs = $true
}
Write-Host ("    ai_summary=" + $ai.summary)

$detail2 = Invoke-Api -Method GET -Path ("/api/alerts/" + $alert2Id) -Headers $auth
$hasAI = $detail2.events | Where-Object { $_.event_type -eq "ai_analysis_requested" }
if (-not $hasAI) { throw "timeline should contain ai_analysis_requested event" }

Write-Host ""
Write-Host "PASS: Alert E2E verification completed"
