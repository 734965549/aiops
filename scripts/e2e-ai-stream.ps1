# AI streaming E2E (requires migration 0049 + running API in dev env).
#
# Covers ops/ai-contract.md §10.17:
#   SSE headers and event order (stage -> answer)
#   archived session -> HTTP 200 + error event FAILED_PRECONDITION (no answer)
#   missing content -> HTTP 400 before any event
#   idempotent replay -> only gathering + answer with the same message_id
#   client abort -> no assistant message, chat_canceled audit with phase
#
# A scripted local mock model is registered as provider `000-e2e-mock`; the
# `[SLOW]` marker delays the model call so the cancel path is deterministic.
#
# Usage:
#   .\scripts\e2e-ai-stream.ps1
#   .\scripts\e2e-ai-stream.ps1 -MockPort 18082

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [int]$MockPort = 18081,
    [string]$MockBaseUrl = ""
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/e2e-ai-mock-provider.ps1"

function Step([string]$Message) {
    Write-Host "==> $Message"
}

function Write-Utf8File([string]$Path, [string]$Content) {
    [System.IO.File]::WriteAllText($Path, $Content, (New-Object System.Text.UTF8Encoding($false)))
}

function Read-SseEvents([string]$Path) {
    $events = New-Object System.Collections.ArrayList
    if (-not (Test-Path $Path)) { return @() }
    foreach ($line in Get-Content -Path $Path -Encoding UTF8) {
        if ($line -like "data: *") {
            $raw = $line.Substring(6)
            try { $null = $events.Add(($raw | ConvertFrom-Json)) } catch { }
        }
    }
    return @($events)
}

function Invoke-StreamRequest {
    param(
        [Parameter(Mandatory = $true)][string]$Curl,
        [Parameter(Mandatory = $true)][string]$Url,
        [Parameter(Mandatory = $true)][string]$Token,
        [Parameter(Mandatory = $true)][string]$BodyFile,
        [Parameter(Mandatory = $true)][string]$HeadersFile,
        [Parameter(Mandatory = $true)][string]$EventsFile
    )
    & $Curl -sS -N --max-time 120 -D $HeadersFile -o $EventsFile -X POST $Url `
        -H "Authorization: Bearer $Token" `
        -H "Content-Type: application/json; charset=utf-8" `
        -H "Accept: text/event-stream" `
        --data-binary "@$BodyFile"
    if ($LASTEXITCODE -ne 0) {
        throw ("curl stream failed with exit code " + $LASTEXITCODE)
    }
}

function Wait-ForText([string]$Path, [string]$Needle, [int]$TimeoutMs = 20000) {
    $deadline = (Get-Date).AddMilliseconds($TimeoutMs)
    while ((Get-Date) -lt $deadline) {
        if (Test-Path $Path) {
            try {
                $text = Get-Content -Path $Path -Raw -Encoding UTF8
                if ($text -and $text.Contains($Needle)) { return $true }
            } catch { }
        }
        Start-Sleep -Milliseconds 100
    }
    return $false
}

$curl = (Get-Command curl.exe -ErrorAction SilentlyContinue).Source
if (-not $curl) {
    throw "curl.exe is required for streaming E2E (Windows 10+ ships it by default)"
}

Write-Host "== AI streaming E2E =="
Step "login as $Username"
$login = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }
$token = [string]$login.access_token

$mock = $null
$tempDir = Join-Path $env:TEMP ("aiops-ai-stream-" + [guid]::NewGuid().ToString("N"))
$null = New-Item -ItemType Directory -Path $tempDir -Force

try {
    if ([string]::IsNullOrWhiteSpace($MockBaseUrl)) {
        Step "start scripted mock model on port $MockPort"
        $mock = Start-AiMockProvider -Port $MockPort
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -Port $MockPort | Out-Null
    } else {
        Step "use external mock model at $MockBaseUrl"
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -BaseUrl $MockBaseUrl | Out-Null
    }

    $context = @{ page = "stream-e2e"; page_title = "流式 E2E"; time_range = "30m" }

    Step "normal stream: headers + event order"
    $session = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E 流式会话"
        context = $context
    }
    $sessionId = [string]$session.session_id
    $bodyFile = Join-Path $tempDir "normal-body.json"
    $headersFile = Join-Path $tempDir "normal-headers.txt"
    $eventsFile = Join-Path $tempDir "normal-events.txt"
    Write-Utf8File $bodyFile (@{ content = "请分析当前告警并给出处置建议。"; context = $context } | ConvertTo-Json -Depth 8 -Compress)
    Invoke-StreamRequest -Curl $curl -Url ("$ApiBase/api/ai/chat/sessions/$sessionId/messages/stream") -Token $token `
        -BodyFile $bodyFile -HeadersFile $headersFile -EventsFile $eventsFile

    $headerText = Get-Content -Path $headersFile -Raw -Encoding UTF8
    if ($headerText -notmatch "(?im)^Content-Type:\s*text/event-stream") { throw "missing text/event-stream content type" }
    if ($headerText -notmatch "(?im)^Cache-Control:\s*no-cache") { throw "missing Cache-Control: no-cache" }
    if ($headerText -notmatch "(?im)^X-Accel-Buffering:\s*no") { throw "missing X-Accel-Buffering: no" }

    $events = Read-SseEvents $eventsFile
    if ($events.Count -lt 4) { throw ("expected >=4 events, got " + $events.Count) }
    if ($events[0].type -ne "stage" -or $events[0].stage -ne "gathering") {
        throw ("first event must be stage:gathering, got: " + ($events[0] | ConvertTo-Json -Compress))
    }
    $stages = @($events | Where-Object { $_.type -eq "stage" } | ForEach-Object { $_.stage })
    if (@($stages) -notcontains "thinking" -or @($stages) -notcontains "answering") {
        throw ("missing thinking/answering stages: " + ($stages -join ","))
    }
    $answerEvent = @($events | Where-Object { $_.type -eq "answer" })
    if ($answerEvent.Count -ne 1) { throw ("expected exactly one answer event, got " + $answerEvent.Count) }
    if (-not $answerEvent[0].result.message_id) { throw "answer event missing message_id" }
    if ($events[-1].type -ne "answer") { throw ("last event must be answer, got " + $events[-1].type) }
    $normalDetail = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/chat/sessions/" + $sessionId) -Headers $auth
    if (@($normalDetail.messages | Where-Object { $_.role -eq "assistant" }).Count -ne 1) {
        throw "normal stream must persist exactly one assistant message"
    }
    Write-Host ("    events=" + $events.Count + " stages=" + ($stages -join " -> "))

    Step "archived session: HTTP 200 + error event, no answer"
    $archivedSession = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{ title = "E2E 已归档流式会话" }
    Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $archivedSession.session_id + "/archive") -Headers $auth | Out-Null
    $bodyFile = Join-Path $tempDir "archived-body.json"
    $headersFile = Join-Path $tempDir "archived-headers.txt"
    $eventsFile = Join-Path $tempDir "archived-events.txt"
    Write-Utf8File $bodyFile (@{ content = "归档后还能继续吗？" } | ConvertTo-Json -Compress)
    Invoke-StreamRequest -Curl $curl -Url ("$ApiBase/api/ai/chat/sessions/" + $archivedSession.session_id + "/messages/stream") -Token $token `
        -BodyFile $bodyFile -HeadersFile $headersFile -EventsFile $eventsFile
    $archivedEvents = Read-SseEvents $eventsFile
    $errorEvents = @($archivedEvents | Where-Object { $_.type -eq "error" })
    if ($errorEvents.Count -ne 1 -or [string]$errorEvents[0].code -ne "FAILED_PRECONDITION") {
        throw ("expected FAILED_PRECONDITION error event, got: " + ($archivedEvents | ConvertTo-Json -Compress))
    }
    if (@($archivedEvents | Where-Object { $_.type -eq "answer" }).Count -ne 0) {
        throw "archived stream must not emit an answer event"
    }
    Write-Host "    archived stream returned error event (FAILED_PRECONDITION)"

    Step "missing content: HTTP 400 before any event"
    $missingSession = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{ title = "E2E 缺参会话" }
    try {
        Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $missingSession.session_id + "/messages/stream") -Headers $auth -Body @{} | Out-Null
        throw "missing content must fail with 400"
    } catch {
        if ($_.Exception.Message -notmatch "INVALID_ARGUMENT") {
            throw ("expected INVALID_ARGUMENT for missing content, got: " + $_.Exception.Message)
        }
    }
    Write-Host "    missing content rejected with INVALID_ARGUMENT"

    Step "idempotent replay: gathering + answer with the first message_id"
    $idemKey = "e2e-stream-" + [guid]::NewGuid().ToString("N")
    $idemBody = @{ content = "幂等重放测试：请分析当前告警。"; context = $context; client_message_id = $idemKey }
    $first = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body $idemBody
    Assert-AiAnswerNotDegraded -Result $first -Context "idempotent sync send"
    $bodyFile = Join-Path $tempDir "idem-body.json"
    $headersFile = Join-Path $tempDir "idem-headers.txt"
    $eventsFile = Join-Path $tempDir "idem-events.txt"
    Write-Utf8File $bodyFile ($idemBody | ConvertTo-Json -Depth 8 -Compress)
    Invoke-StreamRequest -Curl $curl -Url ("$ApiBase/api/ai/chat/sessions/$sessionId/messages/stream") -Token $token `
        -BodyFile $bodyFile -HeadersFile $headersFile -EventsFile $eventsFile
    $idemEvents = Read-SseEvents $eventsFile
    if ($idemEvents.Count -ne 2) { throw ("replay must emit exactly 2 events, got " + $idemEvents.Count) }
    if ($idemEvents[0].type -ne "stage" -or $idemEvents[0].stage -ne "gathering") { throw "replay first event must be gathering" }
    if ($idemEvents[1].type -ne "answer") { throw "replay second event must be answer" }
    if ([string]$idemEvents[1].result.message_id -ne [string]$first.message_id) {
        throw ("replay message_id mismatch: " + $idemEvents[1].result.message_id + " vs " + $first.message_id)
    }
    $idemDetail = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/chat/sessions/" + $sessionId) -Headers $auth
    $idemAssistants = @($idemDetail.messages | Where-Object { $_.role -eq "assistant" })
    if ($idemAssistants.Count -ne 2) {
        throw ("replay must not append assistant messages: expected 2 got " + $idemAssistants.Count)
    }
    Write-Host ("    replay message_id=" + $idemEvents[1].result.message_id)

    Step "client abort: no assistant message + chat_canceled audit"
    $cancelSession = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E 流式取消会话"
        context = $context
    }
    $cancelBodyFile = Join-Path $tempDir "cancel-body.json"
    $cancelEventsFile = Join-Path $tempDir "cancel-events.txt"
    $cancelHeadersFile = Join-Path $tempDir "cancel-headers.txt"
    Write-Utf8File $cancelBodyFile (@{ content = "请分析当前告警并给出处置建议。 [SLOW]"; context = $context } | ConvertTo-Json -Depth 8 -Compress)
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $curl
    $psi.Arguments = "-sS -N --max-time 120 -D `"$cancelHeadersFile`" -o `"$cancelEventsFile`" -X POST `"$ApiBase/api/ai/chat/sessions/$($cancelSession.session_id)/messages/stream`" -H `"Authorization: Bearer $token`" -H `"Content-Type: application/json; charset=utf-8`" -H `"Accept: text/event-stream`" --data-binary `"@$cancelBodyFile`""
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $curlProc = [System.Diagnostics.Process]::Start($psi)
    if (-not (Wait-ForText -Path $cancelEventsFile -Needle "gathering" -TimeoutMs 20000)) {
        try { $curlProc.Kill() } catch { }
        throw "stream did not emit gathering stage before cancel timeout"
    }
    try { $curlProc.Kill() } catch { }
    $null = $curlProc.WaitForExit(5000)

    $cancelAudit = $null
    for ($i = 0; $i -lt 50; $i++) {
        $audits = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $cancelSession.session_id + "&page=1&page_size=50") -Headers $auth
        $cancelAudit = @($audits.items | Where-Object { $_.action -eq "chat_canceled" })[0]
        if ($cancelAudit) { break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $cancelAudit) { throw "missing chat_canceled audit after client abort" }
    if (-not $cancelAudit.payload.phase -or $cancelAudit.payload.result -ne "canceled") {
        throw ("unexpected chat_canceled payload: " + ($cancelAudit.payload | ConvertTo-Json -Compress))
    }
    $cancelDetail = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/chat/sessions/" + $cancelSession.session_id) -Headers $auth
    if (@($cancelDetail.messages | Where-Object { $_.role -eq "assistant" }).Count -ne 0) {
        throw "canceled stream must not persist an assistant message"
    }
    if (@($cancelDetail.messages | Where-Object { $_.role -eq "user" }).Count -ne 1) {
        throw "canceled stream must keep exactly one user message"
    }
    Write-Host ("    canceled phase=" + $cancelAudit.payload.phase)

    Write-Host ""
    Write-Host "PASS: AI streaming E2E"
    Write-Host ("  session_id=" + $sessionId)
}
finally {
    Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    Unregister-AiMockProvider -ApiBase $ApiBase -Headers $auth
    Stop-AiMockProvider $mock
}
