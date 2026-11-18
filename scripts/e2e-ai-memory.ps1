# AI memory E2E (stage 1: similar-case memory, requires migration 0051 + running API + type b provider).
#
# Covers the stage-1 loop:
#   ask -> valid feedback -> case persisted -> ask again -> memory_case reference
#   -> two invalid feedbacks -> case retired -> no memory_case reference.
#
# Usage:
#   .\scripts\e2e-ai-memory.ps1
#   .\scripts\e2e-ai-memory.ps1 -AlertId <alert_id>   # optional: attach a real alert context

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
        $params.ContentType = "application/json; charset=utf-8"
        $jsonBody = $Body | ConvertTo-Json -Depth 12 -Compress
        # Windows PowerShell 5.1 may encode string bodies as ASCII, turning Chinese into '?'.
        $params.Body = [System.Text.Encoding]::UTF8.GetBytes($jsonBody)
    }
    # PS 5.1 throws on 4xx/5xx before the body can be parsed; surface the API code for diagnosability.
    try {
        $resp = Invoke-WebRequest @params
    } catch {
        # PS 5.1 keeps the response body in ErrorDetails.Message; fall back to the raw stream.
        $body = ""
        if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
            $body = $_.ErrorDetails.Message
        } elseif ($_.Exception.Response) {
            try {
                $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                $body = $reader.ReadToEnd()
            } catch { $body = "" }
        }
        $code = ""
        $detail = $_.Exception.Message
        if ($body) {
            try {
                $j = $body | ConvertFrom-Json
                $code = [string]$j.code
                $detail = [string]$j.message
            } catch { }
        }
        throw "API $Method $Path failed: $code $detail"
    }
    $json = $resp.Content | ConvertFrom-Json
    if ($json.code -ne "OK") {
        throw "API $Method $Path failed: $($json.code) $($json.message)"
    }
    return $json.data
}

function Step([string]$Message) {
    Write-Host "==> $Message"
}

function MemoryCaseIds($references) {
    if (-not $references) { return @() }
    return @($references | Where-Object { $_.type -eq "memory_case" } | ForEach-Object { $_.id })
}

Write-Host "== AI memory E2E (stage 1) =="
Step "login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

if (-not $AlertId) {
    Step "find latest active alert (optional context)"
    try {
        $list = Invoke-Api -Method GET -Path "/api/alerts?page=1&page_size=1&active_only=true" -Headers $auth
        if ($list.items -and $list.items.Count -gt 0) {
            $AlertId = $list.items[0].id
        }
    } catch {
        Write-Host "    warn: alert list unavailable, continue without alert context"
    }
}
$contextPayload = @{ page = "memory-e2e"; page_title = "AI 记忆 E2E"; time_range = "30m" }
if ($AlertId) {
    $contextPayload.alert_id = $AlertId
    Write-Host ("    alert_id=" + $AlertId)
} else {
    Write-Host "    no alert context; memory matching falls back to keywords"
}

$question = "分析当前告警的可能原因，并给出处理建议。"
Step "create chat session"
$session = Invoke-Api -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
    title = "E2E AI 记忆会话"
    context = $contextPayload
}

Step "ask once (creates the answer that will be remembered)"
$first = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body @{
    content = $question
    context = $contextPayload
}
if ($first.status -eq "degraded") {
    Write-Host ("SKIP: model degraded (" + $first.answer.degraded_reason + ")")
    Write-Host "      degraded answers are never remembered; configure a type b provider and re-run."
    exit 0
}
if (-not $first.answer.summary) {
    throw "answer.summary is empty"
}

Step "submit valid feedback (case must be persisted)"
$feedback = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages/" + $first.message_id + "/feedback") -Headers $auth -Body @{ feedback = "valid" }
if ($feedback.feedback -ne "valid") {
    throw "feedback not persisted"
}

Step "ask again (memory_case reference expected)"
$second = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body @{
    content = $question
    context = $contextPayload
}
$firstCaseIds = MemoryCaseIds $second.references
if ($firstCaseIds.Count -lt 1) {
    throw "expected memory_case reference after valid feedback, got: $($second.references | ConvertTo-Json -Compress)"
}
$caseId = $firstCaseIds[0]
Write-Host ("    case_id=" + $caseId + " references=" + $firstCaseIds.Count)

Step "two invalid feedbacks retire the case"
Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages/" + $first.message_id + "/feedback") -Headers $auth -Body @{ feedback = "invalid" } | Out-Null
Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages/" + $first.message_id + "/feedback") -Headers $auth -Body @{ feedback = "invalid" } | Out-Null

Step "ask again (no memory_case reference expected)"
$third = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body @{
    content = $question
    context = $contextPayload
}
$remaining = MemoryCaseIds $third.references
if ($remaining -contains $caseId) {
    throw ("retired case still matched: " + $caseId)
}

Step "audit carries memory fields"
$audits = Invoke-Api -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $session.session_id + "&page=1&page_size=100") -Headers $auth
$chatRows = @($audits.items | Where-Object { $_.action -eq "chat_message" })
if ($chatRows.Count -lt 1) {
    throw "audit missing chat_message"
}
$withMemory = @($chatRows | Where-Object { $_.payload.memory_status -ne $null })
if ($withMemory.Count -lt 1) {
    throw "audit chat_message missing memory_status"
}
$memoryUsed = @($withMemory | ForEach-Object { [int]$_.payload.memory_used })
if (-not ($memoryUsed -contains 1)) {
    throw ("expected at least one audit row with memory_used=1, got: " + ($memoryUsed -join ","))
}

Write-Host ""
Write-Host "PASS: AI memory E2E (stage 1)"
Write-Host ("  session_id=" + $session.session_id)
Write-Host ("  case_id=" + $caseId)
Write-Host ("  memory_used=" + ($memoryUsed -join ","))
