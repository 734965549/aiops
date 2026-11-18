# AI knowledge E2E (requires migration 0052 + 0054, running API in dev env).
#
# Covers:
#   draft knowledge -> not injected; activate -> injected with reference + hit_count
#   -> valid feedback increments useful_count -> disable stops injection
#   -> feedback aggregation from 3 memory cases -> draft item
#   -> delete items and retire the generated memory cases.
#
# A scripted local mock model is registered as provider `000-e2e-mock` for the
# duration of the test; app.env must be dev (or AI outbound_allow_loopback=true).
#
# Usage:
#   .\scripts\e2e-ai-knowledge.ps1
#   .\scripts\e2e-ai-knowledge.ps1 -MockPort 18082

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

function KnowledgeRefIds($references) {
    if (-not $references) { return @() }
    return @($references | Where-Object { $_.type -eq "knowledge_item" } | ForEach-Object { $_.id })
}

function Find-KnowledgeItem($items, [string]$ItemId) {
    return @($items | Where-Object { $_.item_id -eq $ItemId })[0]
}

Write-Host "== AI knowledge E2E =="
Step "login as $Username"
$login = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

$mock = $null
$createdItemIds = New-Object System.Collections.ArrayList
$createdCaseIds = New-Object System.Collections.ArrayList

try {
    if ([string]::IsNullOrWhiteSpace($MockBaseUrl)) {
        Step "start scripted mock model on port $MockPort"
        $mock = Start-AiMockProvider -Port $MockPort
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -Port $MockPort | Out-Null
    } else {
        Step "use external mock model at $MockBaseUrl"
        Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -BaseUrl $MockBaseUrl | Out-Null
    }

    $suffix = (Get-Date).ToString("yyyyMMddHHmmss")
    $appId = "e2e-knowledge-app-$suffix"
    $title = "E2E 知识条目 $suffix"
    $context = @{ page = "knowledge-e2e"; page_title = "知识管理 E2E"; application_id = $appId; time_range = "30m" }

    Step "create draft knowledge item"
    $item = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/knowledge/items" -Headers $auth -Body @{
        scope = "application"
        scope_ref = $appId
        title = $title
        content = "E2E 知识：处理磁盘告警时先检查 inode 与日志轮转，再评估扩容。"
    }
    if ($item.status -ne "draft" -or -not $item.item_id) {
        throw ("expected draft item, got: " + ($item | ConvertTo-Json -Compress))
    }
    $itemId = [string]$item.item_id
    $null = $createdItemIds.Add($itemId)
    Write-Host ("    item_id=" + $itemId)

    Step "draft must not be injected"
    $session = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E 知识管理会话 $suffix"
        context = $context
    }
    $sessionId = [string]$session.session_id
    $draftAsk = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "请给出该应用当前告警的处置建议。"
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $draftAsk -Context "draft ask"
    $draftRefs = KnowledgeRefIds $draftAsk.references
    if ($draftRefs -contains $itemId) {
        throw ("draft knowledge was injected: " + $itemId)
    }
    $draftAudit = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $sessionId + "&page=1&page_size=50") -Headers $auth
    $draftRows = @($draftAudit.items | Where-Object { $_.action -eq "chat_message" -and $_.payload.knowledge_status -ne $null })
    if ($draftRows.Count -lt 1) { throw "draft ask audit missing knowledge_status" }
    Write-Host "    draft not injected"

    Step "activate item and expect injection + hit_count"
    $active = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/knowledge/items/" + $itemId + "/review") -Headers $auth -Body @{ action = "activate" }
    if ($active.status -ne "active") { throw ("activate returned status " + $active.status) }

    $activeAsk = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "再次给出该应用当前告警的处置建议。"
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $activeAsk -Context "active ask"
    $activeRefs = KnowledgeRefIds $activeAsk.references
    if (-not ($activeRefs -contains $itemId)) {
        throw ("expected knowledge_item reference " + $itemId + ", got: " + ($activeRefs -join ","))
    }
    $activeAudit = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $sessionId + "&page=1&page_size=50") -Headers $auth
    $activeRows = @($activeAudit.items | Where-Object { $_.action -eq "chat_message" -and [int]$_.payload.knowledge_used -ge 1 })
    if ($activeRows.Count -lt 1) { throw "active ask audit missing knowledge_used>=1" }

    $listed = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/knowledge/items?status=active&scope=application&limit=100") -Headers $auth
    $activeItem = Find-KnowledgeItem $listed.items $itemId
    if (-not $activeItem -or [int]$activeItem.hit_count -lt 1) {
        throw ("expected hit_count>=1 for " + $itemId)
    }
    Write-Host ("    injected, hit_count=" + $activeItem.hit_count)

    Step "valid feedback increments useful_count"
    $feedback = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages/" + $activeAsk.message_id + "/feedback") -Headers $auth -Body @{ feedback = "valid" }
    if ($feedback.feedback -ne "valid") { throw "feedback not persisted" }
    $listed = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/knowledge/items?status=active&scope=application&limit=100") -Headers $auth
    $usefulItem = Find-KnowledgeItem $listed.items $itemId
    if (-not $usefulItem -or [int]$usefulItem.useful_count -lt 1) {
        throw ("expected useful_count>=1 for " + $itemId)
    }
    Write-Host ("    useful_count=" + $usefulItem.useful_count)

    Step "disable item and confirm injection stops"
    $disabled = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/knowledge/items/" + $itemId + "/review") -Headers $auth -Body @{ action = "disable" }
    if ($disabled.status -ne "disabled") { throw ("disable returned status " + $disabled.status) }
    $afterDisable = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = "停用后再给一次建议。"
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $afterDisable -Context "disabled ask"
    if ((KnowledgeRefIds $afterDisable.references) -contains $itemId) {
        throw "disabled knowledge was still injected"
    }

    Step "aggregate feedback (3 memory cases) into a draft"
    $aggSession = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E 知识聚合会话 $suffix"
        context = $context
    }
    for ($i = 1; $i -le 3; $i++) {
        $caseAnswer = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $aggSession.session_id + "/messages") -Headers $auth -Body @{
            content = ("E2E 聚合案例 " + $i + "：分析该应用告警并给出处置建议。")
            context = $context
        }
        Assert-AiAnswerNotDegraded -Result $caseAnswer -Context ("aggregate case " + $i)
        Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $aggSession.session_id + "/messages/" + $caseAnswer.message_id + "/feedback") -Headers $auth -Body @{ feedback = "valid" } | Out-Null
    }

    $cases = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/memory/cases?application_id=" + $appId + "&status=active&limit=20") -Headers $auth
    if (@($cases.items).Count -lt 3) {
        throw ("expected >=3 memory cases for aggregation, got " + @($cases.items).Count)
    }
    foreach ($case in @($cases.items)) { $null = $createdCaseIds.Add([string]$case.case_id) }

    $agg = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/knowledge/aggregate" -Headers $auth -Body @{
        application_id = $appId
        min_cases = 3
    }
    if (@("created", "updated") -notcontains [string]$agg.action) {
        throw ("unexpected aggregate action: " + $agg.action)
    }
    if (-not $agg.item -or $agg.item.origin -ne "feedback_aggregate" -or $agg.item.status -ne "draft") {
        throw ("unexpected aggregate item: " + ($agg.item | ConvertTo-Json -Compress))
    }
    $aggItemId = [string]$agg.item.item_id
    $null = $createdItemIds.Add($aggItemId)
    Write-Host ("    aggregate action=" + $agg.action + " item_id=" + $aggItemId + " source_cases=" + $agg.source_cases)

    Step "delete knowledge items"
    foreach ($id in @($itemId, $aggItemId)) {
        Invoke-AiApi -ApiBase $ApiBase -Method DELETE -Path ("/api/ai/knowledge/items/" + $id) -Headers $auth | Out-Null
        [void]$createdItemIds.Remove($id)
    }
    $remaining = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/ai/knowledge/items?scope=application&limit=100") -Headers $auth
    foreach ($id in @($itemId, $aggItemId)) {
        if (Find-KnowledgeItem $remaining.items $id) {
            throw ("deleted knowledge item still listed: " + $id)
        }
    }

    Step "retire generated memory cases"
    foreach ($caseId in @($createdCaseIds)) {
        Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/memory/cases/" + $caseId + "/status") -Headers $auth -Body @{ status = "retired" } | Out-Null
        [void]$createdCaseIds.Remove($caseId)
    }
    Write-Host "    memory cases retired"

    Write-Host ""
    Write-Host "PASS: AI knowledge E2E"
    Write-Host ("  app_id=" + $appId)
    Write-Host ("  session_id=" + $sessionId)
}
finally {
    foreach ($id in @($createdItemIds)) {
        try { Invoke-AiApi -ApiBase $ApiBase -Method DELETE -Path ("/api/ai/knowledge/items/" + $id) -Headers $auth | Out-Null } catch { }
    }
    foreach ($caseId in @($createdCaseIds)) {
        try { Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/memory/cases/" + $caseId + "/status") -Headers $auth -Body @{ status = "retired" } | Out-Null } catch { }
    }
    Unregister-AiMockProvider -ApiBase $ApiBase -Headers $auth
    Stop-AiMockProvider $mock
}
