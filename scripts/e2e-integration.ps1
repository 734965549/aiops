# Integration account E2E verification (requires PostgreSQL + running API + migrations through 0019).
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-integration.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$ViewerUsername = ("integration_viewer_" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()),
    [string]$ViewerPassword = "IntegrationViewer!12345"
)

$ErrorActionPreference = "Stop"

function New-JsonRequestParams {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = @{},
        $Body = $null
    )
    $params = @{
        Uri = "$($ApiBase.TrimEnd('/'))$Path"
        Method = $Method
        Headers = $Headers
        UseBasicParsing = $true
    }
    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 12 -Compress)
    }
    return $params
}

function ConvertFrom-ApiResponse {
    param($Response)
    $json = $Response.Content | ConvertFrom-Json
    if ($json.code -ne "OK") {
        throw "API returned $($json.code): $($json.message)"
    }
    return $json.data
}

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [hashtable]$Headers = @{},
        $Body = $null
    )
    $params = New-JsonRequestParams -Method $Method -Path $Path -Headers $Headers -Body $Body
    $resp = Invoke-WebRequest @params
    return ConvertFrom-ApiResponse -Response $resp
}

function Invoke-ApiExpectStatus {
    param(
        [string]$Method,
        [string]$Path,
        [int]$ExpectedStatus,
        [hashtable]$Headers = @{},
        $Body = $null
    )
    $params = New-JsonRequestParams -Method $Method -Path $Path -Headers $Headers -Body $Body
    try {
        $resp = Invoke-WebRequest @params
        $status = [int]$resp.StatusCode
        $content = $resp.Content
    } catch {
        $response = $_.Exception.Response
        if ($null -eq $response) {
            throw
        }
        $status = [int]$response.StatusCode
        $content = $_.ErrorDetails.Message
        if ([string]::IsNullOrWhiteSpace($content)) {
            $reader = [System.IO.StreamReader]::new($response.GetResponseStream())
            try {
                $content = $reader.ReadToEnd()
            } finally {
                $reader.Dispose()
            }
        }
    }
    if ($status -ne $ExpectedStatus) {
        throw "API $Method $Path expected HTTP $ExpectedStatus, got $status body=$content"
    }
    if ([string]::IsNullOrWhiteSpace($content)) {
        return $null
    }
    return $content | ConvertFrom-Json
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$adminAuth = @{ Authorization = "Bearer $($login.access_token)" }

Write-Host "==> create huawei_cloud integration account"
$account = Invoke-Api -Method POST -Path "/api/integrations/accounts" -Headers $adminAuth -Body @{
    name = "E2E Huawei Fake"
    provider = "huawei_cloud"
    auth_type = "none"
    regions = @("cn-north-4")
    owner_team = "sre"
    description = "e2e integration account"
}
$accountId = $account.account_id
if (-not $accountId) {
    throw "create account response missing account_id"
}
if ($account.has_credential) {
    throw "auth_type=none without credential must not return has_credential=true"
}
Write-Host ("    account_id=" + $accountId)

Write-Host "==> list and get account"
$list = Invoke-Api -Method GET -Path "/api/integrations/accounts?page=1&page_size=20&provider=huawei_cloud&enabled=true" -Headers $adminAuth
$hit = $list.items | Where-Object { $_.account_id -eq $accountId } | Select-Object -First 1
if (-not $hit) {
    throw "created account not found in list"
}
$detail = Invoke-Api -Method GET -Path ("/api/integrations/accounts/" + $accountId) -Headers $adminAuth
if ($detail.account_id -ne $accountId) {
    throw "account detail mismatch"
}
if ($detail.PSObject.Properties.Name -contains "credential") {
    throw "account response must not expose credential"
}

Write-Host "==> connectivity check"
$check = Invoke-Api -Method POST -Path ("/api/integrations/accounts/" + $accountId + "/check") -Headers $adminAuth
if ($check.status -ne "ok") {
    throw "expected connectivity status ok, got $($check.status)"
}
if (-not $check.capabilities -or ($check.capabilities -notcontains "metrics")) {
    throw "connectivity capabilities should include metrics"
}

Write-Host "==> update account"
$updated = Invoke-Api -Method PUT -Path ("/api/integrations/accounts/" + $accountId) -Headers $adminAuth -Body @{
    name = "E2E Huawei Fake Updated"
    enabled = $true
    regions = @("cn-north-4", "cn-east-3")
    description = "e2e updated"
}
if ($updated.name -ne "E2E Huawei Fake Updated") {
    throw "account update did not persist name"
}

Write-Host "==> create viewer and verify create is forbidden"
$viewer = Invoke-Api -Method POST -Path "/api/identity/admin/users" -Headers $adminAuth -Body @{
    username = $ViewerUsername
    password = $ViewerPassword
    display_name = "E2E Integration Viewer"
    email = "$ViewerUsername@example.com"
}
$roles = Invoke-Api -Method GET -Path "/api/identity/roles?page=1&page_size=100&status=active" -Headers $adminAuth
$viewerRole = $roles.items | Where-Object { $_.code -eq "viewer" } | Select-Object -First 1
if (-not $viewerRole) {
    throw "viewer role not found"
}
Invoke-Api -Method PUT -Path ("/api/identity/admin/users/" + $viewer.id + "/roles") -Headers $adminAuth -Body @{
    role_ids = @($viewerRole.id)
} | Out-Null
$viewerLogin = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $ViewerUsername
    password = $ViewerPassword
}
$viewerAuth = @{ Authorization = "Bearer $($viewerLogin.access_token)" }
$deny = Invoke-ApiExpectStatus -Method POST -Path "/api/integrations/accounts" -ExpectedStatus 403 -Headers $viewerAuth -Body @{
    name = "Denied Integration"
    provider = "prometheus"
    auth_type = "none"
}
if ($deny.code -ne "PERMISSION_DENIED") {
    throw "expected PERMISSION_DENIED, got $($deny.code)"
}

Write-Host "==> delete account"
$deleted = Invoke-Api -Method DELETE -Path ("/api/integrations/accounts/" + $accountId) -Headers $adminAuth
if (-not $deleted.deleted) {
    throw "delete response missing deleted=true"
}

Write-Host ""
Write-Host "PASS: Integration E2E verification completed"
