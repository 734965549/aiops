# AI evidence snapshot E2E (requires migration 0060, local dev API, Docker postgres).
# Verifies the finalized metric observation sent to the mock model has the same
# canonical JSON SHA-256 as obs_evidence_ref.payload.
param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [int]$MockPort = 18083,
    [string]$PgContainer = "aiops-postgres",
    [string]$PgUser = "aiops",
    [string]$PgDatabase = "aiops"
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/e2e-ai-mock-provider.ps1"

function Step([string]$Message) { Write-Host "==> $Message" }

function Get-JsonProperty($Object, [string]$Name) {
    if ($null -eq $Object) { return $null }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) { return $null }
    return $property.Value
}

function Invoke-JsonHash([string]$BaseUrl, $Value) {
    $json = $Value | ConvertTo-Json -Depth 100 -Compress
    $body = [System.Text.Encoding]::UTF8.GetBytes($json)
    return Invoke-RestMethod -Method Post -Uri ($BaseUrl + "/__debug/json-hash") -ContentType "application/json; charset=utf-8" -Body $body
}

function ConvertTo-PgJsonb($Value) {
    $json = $Value | ConvertTo-Json -Depth 100 -Compress
    $tag = "aiops_" + [System.Guid]::NewGuid().ToString("N")
    $delimiter = "$" + $tag + "$"
    $sql = "SELECT " + $delimiter + $json + $delimiter + "::jsonb::text;"
    $previousOutputEncoding = $OutputEncoding
    $OutputEncoding = [System.Text.UTF8Encoding]::new($false)
    try {
        $output = $sql | & docker exec -i $PgContainer psql --username $PgUser --dbname $PgDatabase --no-align --tuples-only --no-psqlrc --quiet --file -
    }
    finally {
        $OutputEncoding = $previousOutputEncoding
    }
    if ($LASTEXITCODE -ne 0) { throw ("JSONB normalization failed: " + ($output -join "`n")) }
    return ($output -join "`n").Trim()
}

Write-Host "== AI evidence snapshot E2E =="
Step "login as $Username"
$login = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

$mock = $null
$accountId = ""
try {
    Step "create fake huawei_cloud account"
    $account = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/integrations/accounts" -Headers $auth -Body @{
        name = "E2E Evidence Snapshot Fake"
        provider = "huawei_cloud"
        auth_type = "none"
        regions = @("cn-north-4")
        owner_team = "sre"
    }
    $accountId = [string]$account.account_id
    if (-not $accountId) { throw "create account response missing account_id" }

    Step "start scripted mock model on port $MockPort"
    $mock = Start-AiMockProvider -Port $MockPort
    Register-AiMockProvider -ApiBase $ApiBase -Headers $auth -Port $MockPort | Out-Null
    $mockBase = "http://127.0.0.1:$MockPort"

    $to = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $from = $to - 30 * 24 * 3600
    $params = @{
        account_id = $accountId
        region = "cn-north-4"
        namespace = "SYS.RDS"
        metric = "cpu_util"
        dimensions = @{ instance_id = "i-1" }
        from = $from
        to = $to
        period = 86400
    }
    $question = "请核对指标证据快照。MOCK_TOOL_RULES " + ($params | ConvertTo-Json -Depth 8 -Compress)
    $context = @{ page = "evidence-e2e"; page_title = "证据快照 E2E"; time_range = "30d" }
    $session = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/chat/sessions" -Headers $auth -Body @{
        title = "E2E 证据快照会话"
        context = $context
    }
    $sessionId = [string]$session.session_id

    Step "collect metric evidence and capture the final model request"
    $sent = Invoke-AiApi -ApiBase $ApiBase -Method POST -Path ("/api/ai/chat/sessions/" + $sessionId + "/messages") -Headers $auth -Body @{
        content = $question
        context = $context
    }
    Assert-AiAnswerNotDegraded -Result $sent -Context "evidence snapshot"
    $audits = Invoke-AiApi -ApiBase $ApiBase -Method GET -Path ("/api/audits?resource_type=ai&resource_id=" + $sessionId + "&page=1&page_size=100") -Headers $auth
    $metricAudit = @($audits.items | Where-Object {
        $_.action -eq "tool_invoke" -and $_.payload.tool_code -eq "metrics.query" -and $_.payload.evidence_id
    })[0]
    if (-not $metricAudit) { throw "metrics.query audit missing evidence_id" }
    $evidenceId = [string]$metricAudit.payload.evidence_id
    if ($evidenceId -notmatch '^ev-[0-9a-f-]+$') { throw ("unexpected evidence_id: " + $evidenceId) }

    $capture = Invoke-RestMethod -Method Get -Uri ($mockBase + "/__debug/last-request")
    $lastUser = @($capture.messages | Where-Object { $_.role -eq "user" })[-1]
    $prompt = [string]$lastUser.content | ConvertFrom-Json
    $observation = @($prompt.observations | Where-Object {
        $_.tool_code -eq "metrics.query" -and [string]$_.data.evidence_id -eq $evidenceId
    })[0]
    if (-not $observation) { throw "model request does not contain the persisted evidence observation" }

    Step "read evidence snapshot from PostgreSQL"
    $sql = "SELECT json_build_object('payload', payload, 'payload_hash', payload_hash, 'query_params', query_params, 'status', status)::text FROM public.obs_evidence_ref WHERE evidence_id = '$evidenceId';"
    $dbOutput = & docker exec $PgContainer psql --username $PgUser --dbname $PgDatabase --no-align --tuples-only --no-psqlrc --quiet --command $sql
    if ($LASTEXITCODE -ne 0) { throw ("psql failed: " + ($dbOutput -join "`n")) }
    $dbRow = ($dbOutput -join "`n") | ConvertFrom-Json
    if (-not $dbRow.payload -or [string]$dbRow.status -ne "ok") {
        throw ("missing ok evidence payload for " + $evidenceId)
    }
    if ([string]$dbRow.payload.schema_version -ne "ai.metric_evidence.v1") {
        throw ("unexpected schema_version: " + $dbRow.payload.schema_version)
    }
    if ([string]$dbRow.query_params.account_id -ne $accountId -or [int64]$dbRow.query_params.from -ne $from -or [int64]$dbRow.query_params.to -ne $to) {
        throw "query_params does not match the metric query"
    }

    $modelEvidence = [ordered]@{}
    foreach ($property in $dbRow.payload.PSObject.Properties) {
        $name = $property.Name
        switch ($name) {
            "original_count" { $modelEvidence[$name] = $observation.data.point_count }
            "retained_count" { $modelEvidence[$name] = $observation.data.retained_point_count }
            "metrics" { $modelEvidence[$name] = @($observation.data.metrics) }
            default { $modelEvidence[$name] = Get-JsonProperty $observation.data $name }
        }
    }

    Step "compare canonical model input and persisted payload hashes"
    $modelJsonb = ConvertTo-PgJsonb $modelEvidence | ConvertFrom-Json
    $modelHash = Invoke-JsonHash -BaseUrl $mockBase -Value $modelJsonb
    $databaseHash = Invoke-JsonHash -BaseUrl $mockBase -Value $dbRow.payload
    if ([string]$modelHash.sha256 -ne [string]$databaseHash.sha256) {
        throw ("model/persisted canonical JSON differs`nmodel=" + $modelHash.canonical + "`ndatabase=" + $databaseHash.canonical)
    }
    if ([string]$databaseHash.sha256 -ne [string]$dbRow.payload_hash) {
        throw ("payload_hash mismatch: computed=" + $databaseHash.sha256 + " stored=" + $dbRow.payload_hash)
    }
    if ([int]$dbRow.payload.original_count -lt [int]$dbRow.payload.retained_count) {
        throw "original_count must be >= retained_count"
    }

    Write-Host ""
    Write-Host "PASS: AI evidence snapshot E2E"
    Write-Host ("  evidence_id=" + $evidenceId)
    Write-Host ("  payload_hash=" + $dbRow.payload_hash)
    Write-Host ("  original_count=" + $dbRow.payload.original_count + " retained_count=" + $dbRow.payload.retained_count)
}
finally {
    Unregister-AiMockProvider -ApiBase $ApiBase -Headers $auth
    Stop-AiMockProvider $mock
    if ($accountId) {
        try {
            Invoke-AiApi -ApiBase $ApiBase -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $auth | Out-Null
        } catch {
            Write-Host ("    warn: remove fake account failed: " + $_.Exception.Message)
        }
    }
}
