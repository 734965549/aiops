# AI assistant E2E (requires migration 0049 + running API + configured AI provider).
#
# Covers the P0 assistant loop:
#   login -> create session -> ask with alert context -> structured answer (risk/actions)
#   -> propose execution task -> confirm -> execute -> audit lookup.
#
# Usage:
#   .\scripts\e2e-ai-assistant.ps1
#   .\scripts\e2e-ai-assistant.ps1 -AlertId <alert_id>   # optional: attach a real alert context

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
    $resp = Invoke-WebRequest @params
    $json = $resp.Content | ConvertFrom-Json
    if ($json.code -ne "OK") {
        throw "API $Method $Path failed: $($json.code) $($json.message)"
    }
    return $json.data
}

function Step([string]$Message) {
    Write-Host "==> $Message"
}

Write-Host "== AI assistant E2E =="
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
if ($AlertId) {
    Write-Host ("    alert_id=" + $AlertId)
} else {
    Write-Host "    no alert context; assistant will answer from page context only"
}

$sessionTitle = "E2E AI 助手会话"
$question = "分析当前页面告警的可能原因，给出风险等级和处置建议；如需变更只生成执行计划。"
$contextPayload = @{ page = "e2e"; page_title = "AI 助手 E2E"; time_range = "30m" }
if ($AlertId) {
    $contextPayload.alert_id = $AlertId
}

Step "create chat session"
$session = Invoke-Api -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
    title = $sessionTitle
    context = $contextPayload
}
if (-not $session.session_id) {
    throw "create session did not return session_id"
}
Write-Host ("    session_id=" + $session.session_id + " status=" + $session.status)

Step "send message (tool checks + model answer)"
$sent = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body @{
    content = $question
    context = $contextPayload
}
$answer = $sent.answer
if (-not $answer -or -not $answer.summary) {
    throw "answer.summary is empty"
}
$answerStatus = [string]$sent.status
Write-Host ("    status=" + $answerStatus + " risk=" + $answer.risk_level + " require_confirm=" + $answer.require_confirm)
if ($answerStatus -eq "degraded") {
    if (-not $answer.degraded_reason) {
        throw "degraded answer must carry degraded_reason"
    }
    Write-Host ("    warn: model degraded (" + $answer.degraded_reason + "); assert structure only")
}
if (@("low", "medium", "high", "info") -notcontains [string]$answer.risk_level) {
    throw ("unexpected risk_level: " + $answer.risk_level)
}
if (-not $answer.actions -or $answer.actions.Count -eq 0) {
    throw "answer.actions is empty"
}
$executionAction = $answer.actions | Where-Object { $_.code -eq "execution.propose" } | Select-Object -First 1
if (-not $executionAction) {
    throw "answer missing execution.propose action"
}
if (-not $executionAction.allowed) {
    throw ("execution.propose denied: " + $executionAction.reason)
}
if ($answer.evidence.Count -eq 0) {
    Write-Host "    warn: answer.evidence is empty"
}

Step "session detail contains persisted answer and UTF-8 user input"
$detail = Invoke-Api -Method GET -Path ("/api/ai/chat/sessions/" + $session.session_id) -Headers $auth
if ($detail.session.title -cne $sessionTitle) {
    throw "session title was not persisted as UTF-8: $($detail.session.title)"
}
$userMessages = @($detail.messages | Where-Object { $_.role -eq "user" })
if ($userMessages.Count -lt 1 -or $userMessages[-1].content -cne $question) {
    throw "user message was not persisted as UTF-8: $($userMessages[-1].content)"
}
$assistantMessages = @($detail.messages | Where-Object { $_.role -eq "assistant" })
if ($assistantMessages.Count -lt 1) {
    throw "session detail has no assistant message"
}
if (-not $assistantMessages[-1].answer.summary) {
    throw "persisted assistant answer summary is empty"
}

Step "propose execution task (source_type=ai_conversation)"
$risk = [string]$answer.risk_level
$body = @{
    name = "E2E AI 建议处置任务"
    source_type = "ai_conversation"
    source_id = $session.session_id
    operation_type = "custom"
    target_type = "resource"
    target_name = "e2e-target"
    parameters = @{
        session_id = $session.session_id
        message_id = $sent.message_id
        summary = $answer.summary
    }
    rollback_plan = @{ description = [string]$answer.rollback }
}
if (@("medium", "high") -contains $risk) {
    $body.risk_level = $risk
}
$created = Invoke-Api -Method POST -Path "/api/executions/tasks" -Headers $auth -Body $body
Write-Host ("    task_id=" + $created.task_id + " status=" + $created.status + " risk=" + $created.risk_level)
if ($created.status -ne "pending_confirm") {
    throw ("expected pending_confirm for AI-proposed custom task, got " + $created.status)
}

Step "confirm task with CONFIRM"
$confirmed = Invoke-Api -Method POST -Path ("/api/executions/tasks/" + $created.task_id + "/confirm") -Headers $auth -Body @{
    confirm = $true
    confirm_text = "CONFIRM"
}
if ($confirmed.status -ne "pending_execute") {
    throw ("expected pending_execute after confirm, got " + $confirmed.status)
}

Step "execute confirmed task"
$executed = Invoke-Api -Method POST -Path ("/api/executions/tasks/" + $created.task_id + "/execute") -Headers $auth -Body @{}
if ($executed.task.status -ne "success") {
    throw ("expected success execution, got " + $executed.task.status)
}

Step "message idempotency (same client_message_id returns first result)"
$idempotentBody = @{
    content = $question
    context = $contextPayload
    client_message_id = "e2e-cm-1"
}
$firstIdem = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body $idempotentBody
$secondIdem = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body $idempotentBody
if ($firstIdem.message_id -ne $secondIdem.message_id) {
    throw ("idempotent replay returned different message_id: " + $firstIdem.message_id + " vs " + $secondIdem.message_id)
}
if ($firstIdem.answer.summary -cne $secondIdem.answer.summary) {
    throw "idempotent replay returned a different answer summary"
}
$idemDetail = Invoke-Api -Method GET -Path ("/api/ai/chat/sessions/" + $session.session_id) -Headers $auth
$idemUserMessages = @($idemDetail.messages | Where-Object { $_.role -eq "user" })
$expectedUserCount = $userMessages.Count + 1
if ($idemUserMessages.Count -ne $expectedUserCount) {
    throw ("idempotent replay must not append user messages: expected " + $expectedUserCount + " got " + $idemUserMessages.Count)
}
Write-Host ("    replayed message_id=" + $secondIdem.message_id + " user_messages=" + $idemUserMessages.Count)

Step "submit valid feedback"
$feedback = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages/" + $sent.message_id + "/feedback") -Headers $auth -Body @{ feedback = "valid" }
if ($feedback.feedback -ne "valid") {
    throw "feedback not persisted"
}

Step "archive session (read-only, history kept)"
$archived = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/archive") -Headers $auth
if ($archived.status -ne "archived") {
    throw ("expected archived status, got " + $archived.status)
}
$archiveDetail = Invoke-Api -Method GET -Path ("/api/ai/chat/sessions/" + $session.session_id) -Headers $auth
if (-not $archiveDetail.messages -or $archiveDetail.messages.Count -lt 2) {
    throw "archived session must keep its history"
}
try {
    Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/messages") -Headers $auth -Body @{ content = "归档后仍可继续？" } | Out-Null
    throw "archived session must reject new messages with 409"
} catch {
    $msg = $_.Exception.Message
    if ($msg -notmatch "FAILED_PRECONDITION") {
        throw ("expected FAILED_PRECONDITION for archived session, got: " + $msg)
    }
    Write-Host "    archived session rejects new messages (FAILED_PRECONDITION)"
}

Step "unarchive session"
$restored = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/unarchive") -Headers $auth
if ($restored.status -ne "active") {
    throw ("expected active status after unarchive, got " + $restored.status)
}

Step "delete an archived probe session (messages removed with session)"
$probe = Invoke-Api -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{ title = "E2E AI 待删除会话" }
$deleted = Invoke-Api -Method DELETE -Path ("/api/ai/chat/sessions/" + $probe.session_id) -Headers $auth
if (-not $deleted.deleted) {
    throw "delete session did not return deleted=true"
}
try {
    Invoke-Api -Method GET -Path ("/api/ai/chat/sessions/" + $probe.session_id) -Headers $auth | Out-Null
    throw "deleted session must not be readable"
} catch {
    if ($_.Exception.Message -notmatch "NOT_FOUND") {
        throw ("expected NOT_FOUND after delete, got: " + $_.Exception.Message)
    }
    Write-Host "    deleted session is gone (NOT_FOUND)"
}

Step "session share (create read-only link, view, revoke)"
$share = Invoke-Api -Method POST -Path ("/api/ai/chat/sessions/" + $session.session_id + "/shares") -Headers $auth -Body @{ expires_in_hours = 24 }
if (-not $share.share_id -or -not $share.token) {
    throw "create share did not return share_id/token"
}
if ($share.share_url -ne ("/ai-assistant/shared/" + $share.token)) {
    throw ("unexpected share_url: " + $share.share_url)
}
$sharedView = Invoke-Api -Method GET -Path ("/api/ai/chat/shares/" + $share.token) -Headers $auth
if (-not $sharedView.session -or -not $sharedView.messages) {
    throw "shared session view missing session/messages"
}
if (@($sharedView.messages).Count -lt 2) {
    throw "shared session view must include history"
}
$revokedShare = Invoke-Api -Method DELETE -Path ("/api/ai/chat/sessions/" + $session.session_id + "/shares/" + $share.share_id) -Headers $auth
if ($revokedShare.status -ne "revoked") {
    throw ("expected revoked share status, got " + $revokedShare.status)
}
try {
    Invoke-Api -Method GET -Path ("/api/ai/chat/shares/" + $share.token) -Headers $auth | Out-Null
    throw "revoked share link must not be readable"
} catch {
    if ($_.Exception.Message -notmatch "NOT_FOUND") {
        throw ("expected NOT_FOUND after revoke, got: " + $_.Exception.Message)
    }
    Write-Host "    revoked share link is gone (NOT_FOUND)"
}

Step "audit records exist for chat and tool invocation"
$audits = Invoke-Api -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $session.session_id + "&page=1&page_size=50") -Headers $auth
$actions = @($audits.items | ForEach-Object { $_.action })
if ($actions -notcontains "chat_message") {
    throw "audit missing chat_message"
}
if ($actions -notcontains "chat_feedback") {
    throw "audit missing chat_feedback"
}
if ($actions -notcontains "tool_invoke") {
    throw "audit missing tool_invoke"
}
if ($actions -notcontains "chat_session_status") {
    throw "audit missing chat_session_status (archive/unarchive)"
}
if ($actions -notcontains "chat_share_create") {
    throw "audit missing chat_share_create"
}
if ($actions -notcontains "chat_share_revoke") {
    throw "audit missing chat_share_revoke"
}
if ($actions -notcontains "chat_share_view") {
    throw "audit missing chat_share_view"
}
$probeAudits = Invoke-Api -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $probe.session_id + "&page=1&page_size=50") -Headers $auth
$probeActions = @($probeAudits.items | ForEach-Object { $_.action })
if ($probeActions -notcontains "chat_session_delete") {
    throw "audit missing chat_session_delete"
}
$replayedAudits = @($idemDetail.messages | Where-Object { $_.role -eq "assistant" })
if ($replayedAudits.Count -ne 2) {
    throw ("idempotency replay must not add assistant messages: expected 2 got " + $replayedAudits.Count)
}
$execAudits = Invoke-Api -Method GET -Path ("/api/audits?resource_type=execution&resource_id=" + $created.task_id + "&page=1&page_size=50") -Headers $auth
$execActions = @($execAudits.items | ForEach-Object { $_.action })
foreach ($required in @("create", "confirm", "execute")) {
    if ($execActions -notcontains $required) {
        throw ("execution audit missing action: " + $required)
    }
}

Write-Host ""
Write-Host "PASS: AI assistant E2E"
Write-Host ("  session_id=" + $session.session_id)
Write-Host ("  message_id=" + $sent.message_id)
Write-Host ("  task_id=" + $created.task_id)
Write-Host ("  answer_status=" + $answerStatus)
