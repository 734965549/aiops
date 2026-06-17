# Runbook E2E: alert -> recommend -> create -> confirm -> execute -> timeline
# Requires PostgreSQL migrations 0011+0012, running API, seed runbook template.
#
# Usage (standalone):
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-runbook.ps1
#
# Usage (reuse existing processing alert):
#   .\scripts\e2e-runbook.ps1 -AlertId <alert_id>

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$AlertId = "",
    [string]$SourceId = "e2e-runbook-am",
    [string]$WebhookSecret = "e2e-runbook-webhook-secret",
    [string]$SeedTemplateId = "00000000-0000-0000-0002-000000000001"
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

function Ensure-ProcessingRunbookAlert {
    param(
        [hashtable]$Auth,
        [string]$SourceId,
        [string]$WebhookSecret
    )

    Write-Host "==> ensure alert source $SourceId (prod)"
    try {
        Invoke-Api -Method POST -Path "/api/alerts/sources" -Headers $Auth -Body @{
            id = $SourceId
            name = "E2E Runbook Alertmanager"
            type = "prometheus_alertmanager"
            enabled = $true
            secret = $WebhookSecret
            environment = "prod"
        } | Out-Null
    } catch {
        Write-Host "    source may already exist, continue"
    }

    $fingerprint = "e2e-fp-runbook-001"
    $firingPayload = @{
        status = "firing"
        alerts = @(
            @{
                status = "firing"
                fingerprint = $fingerprint
                labels = @{
                    alertname = "HighCPU"
                    severity = "critical"
                    pod = "payment-pod-1"
                    namespace = "prod"
                }
                annotations = @{
                    summary = "Pod CPU > 90%"
                    description = "E2E runbook test alert"
                }
                startsAt = "2026-06-13T00:00:00Z"
            }
        )
    }

    Write-Host "==> ingest firing alert (HighCPU / pod / prod)"
    $ingest = Invoke-Api -Method POST -Path ("/api/alerts/ingest/alertmanager/" + $SourceId) -Headers @{
        "X-AIOPS-Webhook-Token" = $WebhookSecret
    } -Body $firingPayload
    Write-Host ("    created=" + $ingest.created + " updated=" + $ingest.updated)

    $list = Invoke-Api -Method GET -Path ("/api/alerts?source_id=" + $SourceId + "&active_only=true&keyword=HighCPU") -Headers $Auth
    if (-not $list.items -or $list.items.Count -eq 0) {
        throw "expected active HighCPU alert after ingest"
    }
    $alert = $list.items | Where-Object { $_.name -eq "HighCPU" } | Select-Object -First 1
    if (-not $alert) {
        $alert = $list.items[0]
    }
    $id = $alert.id
    Write-Host ("    alert_id=" + $id + " status=" + $alert.status)

    if ($alert.status -eq "new") {
        Write-Host "==> acknowledge alert"
        Invoke-Api -Method POST -Path ("/api/alerts/" + $id + "/acknowledge") -Headers $Auth -Body @{} | Out-Null
    }
    $detail = Invoke-Api -Method GET -Path ("/api/alerts/" + $id) -Headers $Auth
    if ($detail.alert.status -eq "acknowledged") {
        Write-Host "==> start processing"
        Invoke-Api -Method POST -Path ("/api/alerts/" + $id + "/start-processing") -Headers $Auth -Body @{} | Out-Null
    }
    $detail2 = Invoke-Api -Method GET -Path ("/api/alerts/" + $id) -Headers $Auth
    if ($detail2.alert.status -ne "processing") {
        throw ("expected processing status, got " + $detail2.alert.status)
    }
    if ($detail2.alert.environment -ne "prod") {
        throw ("expected prod environment, got " + $detail2.alert.environment)
    }
    return $id
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

if (-not $AlertId) {
    $AlertId = Ensure-ProcessingRunbookAlert -Auth $auth -SourceId $SourceId -WebhookSecret $WebhookSecret
} else {
    Write-Host ("==> use provided alert_id=" + $AlertId)
    $existing = Invoke-Api -Method GET -Path ("/api/alerts/" + $AlertId) -Headers $auth
    if ($existing.alert.status -ne "processing") {
        throw ("alert must be processing for runbook E2E, got " + $existing.alert.status)
    }
}
Write-Host ("    alert_id=" + $AlertId)

Write-Host "==> fetch runbook recommendations"
$rec = Invoke-Api -Method GET -Path ("/api/runbooks/recommendations?alert_id=" + $AlertId) -Headers $auth
if (-not $rec.items -or $rec.items.Count -eq 0) {
    throw "expected at least one runbook recommendation (migration 0012 seed?)"
}
$pick = $rec.items | Where-Object { $_.template_id -eq $SeedTemplateId } | Select-Object -First 1
if (-not $pick) {
    $pick = $rec.items[0]
}
Write-Host ("    recommended=" + $pick.name + " template_id=" + $pick.template_id + " steps=" + $pick.steps_count)
if ($pick.steps_count -lt 1) {
    throw "recommended runbook should have steps"
}

Write-Host "==> create execution task from runbook (dry-run)"
$created = Invoke-Api -Method POST -Path "/api/executions/tasks" -Headers $auth -Body @{
    source_type = "alert"
    source_id = $AlertId
    runbook_template_id = $pick.template_id
    dry_run = $true
    parameters = @{
        service_name = "payment-service"
        replicas = 3
    }
}
Write-Host ("    task_id=" + $created.task_id + " status=" + $created.status + " risk=" + $created.risk_level)
if ($created.status -ne "pending_confirm") {
    throw "expected pending_confirm for prod runbook task, got $($created.status)"
}

Write-Host "==> verify task detail (multi-step + runbook metadata)"
$before = Invoke-Api -Method GET -Path ("/api/executions/tasks/" + $created.task_id) -Headers $auth
if (-not $before.task.runbook_template_id) {
    throw "task missing runbook_template_id"
}
if (-not $before.task.dry_run) {
    throw "expected task dry_run=true"
}
if ($before.steps.Count -lt 2) {
    throw ("expected multiple exec steps from runbook, got " + $before.steps.Count)
}
Write-Host ("    steps=" + $before.steps.Count + " runbook=" + $before.task.runbook_template_id)

Write-Host "==> verify timeline after create"
$afterCreate = Invoke-Api -Method GET -Path ("/api/alerts/" + $AlertId) -Headers $auth
$createdEv = $afterCreate.events | Where-Object {
    $_.event_type -eq "execution_created" -and $_.payload.execution_id -eq $created.task_id
} | Select-Object -First 1
if (-not $createdEv) {
    throw "timeline missing execution_created for current task"
}
if (-not $createdEv.payload.runbook_template_id) {
    throw "execution_created payload missing runbook_template_id"
}
Write-Host ("    execution_created runbook=" + $createdEv.payload.runbook_template_id)

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
    throw ("expected success, got " + $detail.task.status + " err=" + $detail.task.error_message)
}
$failedSteps = @($detail.steps | Where-Object { $_.status -eq "failed" })
if ($failedSteps.Count -gt 0) {
    throw ("unexpected failed steps: " + ($failedSteps | ForEach-Object { $_.name }) -join ", ")
}
Write-Host ("    result=" + $detail.task.result_summary)

Write-Host "==> verify dry-run output on first step"
$step1 = $detail.steps | Sort-Object { [int]$_.step_order } | Select-Object -First 1
if (-not $step1.output.dry_run) {
    throw "expected dry_run marker in step output"
}

Write-Host "==> verify alert timeline (created / started / finished)"
$alertFinal = Invoke-Api -Method GET -Path ("/api/alerts/" + $AlertId) -Headers $auth
$types = @($alertFinal.events | ForEach-Object { $_.event_type })
foreach ($t in @("execution_created", "execution_started", "execution_finished")) {
    if ($types -notcontains $t) {
        throw "alert timeline missing event: $t"
    }
}
$finishedEv = $alertFinal.events | Where-Object { $_.event_type -eq "execution_finished" } | Select-Object -First 1
if ($finishedEv.payload.status -ne "success") {
    throw ("execution_finished expected success, got " + $finishedEv.payload.status)
}
Write-Host ("    timeline events OK, finished status=" + $finishedEv.payload.status)

Write-Host ""
Write-Host "PASS: Runbook E2E verification completed"
