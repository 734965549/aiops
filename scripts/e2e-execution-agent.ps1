# Execution Agent E2E verification (requires PostgreSQL + running API + migrations through 0022).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-execution-agent.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$RegisterToken = $(if ($env:EXEC_AGENT_REGISTER_TOKEN) { $env:EXEC_AGENT_REGISTER_TOKEN } else { "dev-agent-register-token" })
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

function Invoke-ApiExpectFail {
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
    try {
        $resp = Invoke-WebRequest @params
        $json = $resp.Content | ConvertFrom-Json
        if ($json.code -eq "OK") {
            throw "expected failure but got OK for $Method $Path"
        }
        return $json
    } catch {
        if ($_.Exception.Response) {
            $raw = $_.ErrorDetails.Message
            if ([string]::IsNullOrWhiteSpace($raw)) {
                $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                try {
                    $raw = $reader.ReadToEnd()
                } finally {
                    $reader.Dispose()
                }
            }
            return ($raw | ConvertFrom-Json)
        }
        throw
    }
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$auth = @{ Authorization = "Bearer $($login.access_token)" }

Write-Host "==> create execution medium"
$medium = Invoke-Api -Method POST -Path "/api/executions/media" -Headers $auth -Body @{
    medium_id = "med-e2e-jumpbox-01"
    name = "E2E Jumpbox"
    medium_type = "jumpbox"
    environment = "prod"
    region = "cn-north-4"
    network_zone = "prod-vpc-a"
    capabilities = @("linux.command.readonly")
    enabled = $true
}
$mediumId = $medium.medium_id
Write-Host ("    medium_id=" + $mediumId)

Write-Host "==> register fake execution agent"
$agent = Invoke-Api -Method POST -Path "/api/executions/agents/register" -Headers @{ "X-Register-Token" = $RegisterToken } -Body @{
    agent_id = "agent-e2e-jumpbox-01"
    medium_id = $mediumId
    version = "0.1.0-e2e"
    capabilities = @("linux.command.readonly")
}
$agentAuth = @{ Authorization = "Bearer $($agent.agent_token)" }
Write-Host ("    agent_id=" + $agent.agent_id)

Write-Host "==> agent heartbeat"
Invoke-Api -Method POST -Path "/api/executions/agents/$($agent.agent_id)/heartbeat" -Headers $agentAuth -Body @{
    status = "online"
    running_tasks = 0
    free_slots = 2
    version = "0.1.0-e2e"
} | Out-Null

Write-Host "==> create agent-mode execution task"
$task = Invoke-Api -Method POST -Path "/api/executions/tasks" -Headers $auth -Body @{
    name = "E2E disk check"
    source_type = "manual"
    operation_type = "command"
    environment = "prod"
    execution_mode = "agent"
    medium_id = $mediumId
    command_spec_id = "cmd_linux_disk_usage"
    arguments = @{ mount_point = "/" }
    risk_level = "high"
}
$taskId = $task.task_id
Write-Host ("    task_id=" + $taskId + " status=" + $task.status)

Write-Host "==> pending_confirm task must not be leased"
$emptyLease = Invoke-Api -Method POST -Path "/api/executions/agents/$($agent.agent_id)/lease" -Headers $agentAuth -Body @{}
if ($emptyLease.task_id) {
    throw "agent leased task before confirm"
}
Write-Host "    lease empty as expected"

Write-Host "==> confirm task"
$confirmed = Invoke-Api -Method POST -Path "/api/executions/tasks/$taskId/confirm" -Headers $auth -Body @{
    confirm = $true
    confirm_text = "CONFIRM"
}
if ($confirmed.status -ne "pending_execute") {
    throw "expected pending_execute after confirm, got $($confirmed.status)"
}

Write-Host "==> agent lease task"
$lease = Invoke-Api -Method POST -Path "/api/executions/agents/$($agent.agent_id)/lease" -Headers $agentAuth -Body @{}
if (-not $lease.lease_id -or -not $lease.task_id) {
    throw "expected lease payload"
}
Write-Host ("    lease_id=" + $lease.lease_id)

Write-Host "==> append stdout log"
Invoke-Api -Method POST -Path "/api/executions/agents/$($agent.agent_id)/tasks/$taskId/logs" -Headers $agentAuth -Body @{
    lease_id = $lease.lease_id
    step_id = $lease.step_id
    stream = "stdout"
    sequence = 1
    content = "Filesystem Size Used Avail Use% Mounted on`n/dev/root 40G 24G 16G 61% /"
} | Out-Null

Write-Host "==> report success result"
$result = Invoke-Api -Method POST -Path "/api/executions/agents/$($agent.agent_id)/tasks/$taskId/result" -Headers $agentAuth -Body @{
    lease_id = $lease.lease_id
    step_id = $lease.step_id
    status = "success"
    exit_code = 0
    result_summary = "根分区使用率 61%"
}
if ($result.task.status -ne "success") {
    throw "expected task success, got $($result.task.status)"
}

Write-Host "==> invalid arguments rejected"
$bad = Invoke-ApiExpectFail -Method POST -Path "/api/executions/tasks" -Headers $auth -Body @{
    name = "bad task"
    source_type = "manual"
    operation_type = "command"
    execution_mode = "agent"
    medium_id = $mediumId
    command_spec_id = "cmd_linux_disk_usage"
    arguments = @{ mount_point = "/; rm -rf /" }
    risk_level = "high"
}
if ($bad.code -ne "INVALID_ARGUMENT") {
    throw "expected INVALID_ARGUMENT for bad arguments"
}

Write-Host "==> E2E execution agent passed"
