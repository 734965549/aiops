# Execution Agent permission negative checks.
param(
    [string]$ApiBase = $(if ($env:API_BASE) { $env:API_BASE } else { "http://127.0.0.1:8080" }),
    [string]$Username = "admin",
    [string]$Password = "admin123"
)

$ErrorActionPreference = "Stop"

function Invoke-ApiRaw {
    param([string]$Method, [string]$Path, [hashtable]$Headers = @{}, $Body = $null)
    $uri = "$($ApiBase.TrimEnd('/'))$Path"
    $params = @{ Uri = $uri; Method = $Method; Headers = $Headers; UseBasicParsing = $true }
    if ($null -ne $Body) {
        $params.ContentType = "application/json"
        $params.Body = ($Body | ConvertTo-Json -Depth 8 -Compress)
    }
    try {
        $resp = Invoke-WebRequest @params
        return ($resp.Content | ConvertFrom-Json)
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

$login = Invoke-ApiRaw -Method POST -Path "/api/identity/login" -Body @{ username = $Username; password = $Password }
if ($login.code -ne "OK") { throw "login failed" }
$auth = @{ Authorization = "Bearer $($login.data.access_token)" }

$deny = Invoke-ApiRaw -Method POST -Path "/api/executions/media" -Headers $auth -Body @{
    name = "should fail"; medium_type = "jumpbox"
}
# admin has permission; this script mainly verifies agent register token gate
$badRegister = Invoke-ApiRaw -Method POST -Path "/api/executions/agents/register" -Headers @{ "X-Register-Token" = "wrong-token" } -Body @{
    medium_id = "med-missing"; agent_id = "agent-bad"
}
if ($badRegister.code -ne "PERMISSION_DENIED") {
    throw "expected PERMISSION_DENIED for invalid register token, got $($badRegister.code)"
}

Write-Host "==> E2E execution agent permission checks passed"
