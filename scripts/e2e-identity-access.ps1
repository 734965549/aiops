# Identity access-control E2E.
# Requires API + PostgreSQL + migrations through 0015.
#
# Usage:
#   docker compose -f deployments/docker-compose.yml -f deployments/docker-compose.dev.yml up -d
#   go run ./cmd/migrate
#   .\scripts\e2e-identity-access.ps1

param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123",
    [string]$TestUsername = ("viewer_e2e_" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()),
    [string]$TestPassword = "ViewerE2e!12345"
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
    try {
        return $content | ConvertFrom-Json
    } catch {
        return $null
    }
}

Write-Host "==> login as $Username"
$login = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $Username
    password = $Password
}
$adminAuth = @{ Authorization = "Bearer $($login.access_token)" }

Write-Host "==> create local viewer test user $TestUsername"
$created = Invoke-Api -Method POST -Path "/api/identity/admin/users" -Headers $adminAuth -Body @{
    username = $TestUsername
    password = $TestPassword
    display_name = "E2E Viewer"
    email = "$TestUsername@example.com"
}
$testUserId = $created.id
if (-not $testUserId) {
    throw "created user response missing id"
}
Write-Host ("    user_id=" + $testUserId)

Write-Host "==> fetch viewer role"
$rolePage = Invoke-Api -Method GET -Path "/api/identity/roles?page=1&page_size=100&status=active" -Headers $adminAuth
$viewer = $rolePage.items | Where-Object { $_.code -eq "viewer" } | Select-Object -First 1
if (-not $viewer) {
    throw "viewer role not found; ensure migration 0015 has been applied"
}
Write-Host ("    viewer_role_id=" + $viewer.id)

Write-Host "==> bind viewer role manually"
$bindings = Invoke-Api -Method PUT -Path ("/api/identity/admin/users/" + $testUserId + "/roles") -Headers $adminAuth -Body @{
    role_ids = @($viewer.id)
}
$viewerBinding = $bindings.items | Where-Object { $_.id -eq $viewer.id -and $_.source -eq "manual" } | Select-Object -First 1
if (-not $viewerBinding) {
    throw "expected manual viewer binding after replace"
}

Write-Host "==> login as viewer test user"
$viewerLogin = Invoke-Api -Method POST -Path "/api/identity/login" -Body @{
    username = $TestUsername
    password = $TestPassword
}
$viewerAuth = @{ Authorization = "Bearer $($viewerLogin.access_token)" }

Write-Host "==> verify viewer can access read-only dashboard"
$summary = Invoke-Api -Method GET -Path "/api/dashboard/summary" -Headers $viewerAuth
if ($null -eq $summary) {
    throw "dashboard summary response missing data"
}

Write-Host "==> verify viewer cannot write access-control bindings"
$deny = Invoke-ApiExpectStatus -Method PUT -Path ("/api/identity/admin/users/" + $testUserId + "/roles") -ExpectedStatus 403 -Headers $viewerAuth -Body @{
    role_ids = @()
}
if ($deny -and $deny.code -and $deny.code -ne "PERMISSION_DENIED") {
    throw "expected PERMISSION_DENIED for viewer write, got $($deny.code)"
}

Write-Host "==> verify operation audit record"
$audits = Invoke-Api -Method GET -Path ("/api/audits?resource_type=identity_user&resource_id=" + $testUserId + "&action=set_user_roles&page=1&page_size=10") -Headers $adminAuth
if (-not $audits.items -or $audits.items.Count -lt 1) {
    throw "expected set_user_roles audit record"
}

Write-Host "==> E2E identity access-control passed"
