# Shared helpers for AI E2E scripts (PowerShell 5.1 compatible).
#
# Provides:
#   Invoke-AiApi            - JSON API call with business-code error surfacing
#   Start-AiMockProvider    - build + start scripts/tools/ai-mock-provider on loopback
#   Stop-AiMockProvider     - stop the mock process and remove temp files
#   Register-AiMockProvider - register the mock as the lowest-id type b provider
#   Unregister-AiMockProvider - delete the reserved mock provider
#
# The mock provider requires app.env=dev (or AI outbound_allow_loopback=true)
# because provider calls to loopback are blocked outside dev.
# Remote-API runs (e.g. against 203): start the mock in the target compose network
# (docker run --network <net> ... -addr 0.0.0.0:18081) and pass its reachable URL
# via -MockBaseUrl on the consumer scripts; the local Start/Stop steps are skipped.

$script:AiMockProviderID = "000-e2e-mock"

function Invoke-AiApi {
    param(
        [Parameter(Mandatory = $true)][string]$ApiBase,
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Path,
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
    try {
        $resp = Invoke-WebRequest @params
    } catch {
        $raw = ""
        if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
            $raw = $_.ErrorDetails.Message
        } elseif ($_.Exception.Response) {
            try {
                $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                $raw = $reader.ReadToEnd()
            } catch { $raw = "" }
        }
        $code = ""
        $detail = $_.Exception.Message
        if ($raw) {
            try {
                $j = $raw | ConvertFrom-Json
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

function Test-AiPortOpen {
    param([Parameter(Mandatory = $true)][int]$Port)
    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $client.Connect("127.0.0.1", $Port)
        return $true
    } catch {
        return $false
    } finally {
        $client.Close()
    }
}

function Start-AiMockProvider {
    param(
        [int]$Port = 18081,
        [string]$SlowDelay = "8s"
    )
    if (Test-AiPortOpen -Port $Port) {
        throw "port $Port is already in use; stop the existing listener first"
    }
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $exe = Join-Path $env:TEMP ("aiops-ai-mock-" + [guid]::NewGuid().ToString("N") + ".exe")
    $stdout = "$exe.stdout.log"
    $stderr = "$exe.stderr.log"
    Push-Location $repoRoot
    try {
        & go build -o $exe ./scripts/tools/ai-mock-provider
        if ($LASTEXITCODE -ne 0) {
            throw "go build ./scripts/tools/ai-mock-provider failed"
        }
    } finally {
        Pop-Location
    }
    $proc = Start-Process -FilePath $exe `
        -ArgumentList @("-addr", "127.0.0.1:$Port", "-slow", $SlowDelay) `
        -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    for ($i = 0; $i -lt 150; $i++) {
        if (Test-AiPortOpen -Port $Port) {
            return [pscustomobject]@{
                Process = $proc
                Exe = $exe
                StdoutLog = $stdout
                StderrLog = $stderr
                Port = $Port
            }
        }
        if ($proc.HasExited) {
            $errText = ""
            if (Test-Path $stderr) { $errText = Get-Content $stderr -Raw }
            throw ("ai mock provider exited early: " + $errText)
        }
        Start-Sleep -Milliseconds 100
    }
    throw "ai mock provider did not start listening on port $Port"
}

function Stop-AiMockProvider {
    param($Mock)
    if ($null -eq $Mock) { return }
    try {
        if ($Mock.Process -and -not $Mock.Process.HasExited) {
            Stop-Process -Id $Mock.Process.Id -Force
        }
    } catch { }
    foreach ($path in @($Mock.Exe, $Mock.StdoutLog, $Mock.StderrLog)) {
        if ($path -and (Test-Path $path)) {
            Remove-Item $path -Force -ErrorAction SilentlyContinue
        }
    }
}

function Register-AiMockProvider {
    param(
        [Parameter(Mandatory = $true)][string]$ApiBase,
        [Parameter(Mandatory = $true)][hashtable]$Headers,
        [int]$Port = 18081,
        # BaseUrl 为空时按本机 loopback 注册；远程 API 场景传入外部可达地址
        # （如 203 compose 网络内的 http://ai-e2e-mock:18081/v1）。
        [string]$BaseUrl = ""
    )
    $providers = @(Invoke-AiApi -ApiBase $ApiBase -Method GET -Path "/api/ai/providers" -Headers $Headers)
    $conflict = $providers | Where-Object { $_.id -eq $script:AiMockProviderID } | Select-Object -First 1
    if ($conflict) {
        throw "reserved provider id '$($script:AiMockProviderID)' already exists; remove it manually before running AI E2E"
    }
    if ([string]::IsNullOrWhiteSpace($BaseUrl)) {
        $BaseUrl = "http://127.0.0.1:$Port/v1"
    }
    Invoke-AiApi -ApiBase $ApiBase -Method POST -Path "/api/ai/providers" -Headers $Headers -Body @{
        id = $script:AiMockProviderID
        name = "E2E Mock Provider"
        type = "b"
        base_url = $BaseUrl
        model = "e2e-mock-model"
        api_key = "e2e-mock-key"
        timeout_ms = 60000
        enabled = $true
        description = "Temporary provider used by scripts/e2e-ai-*.ps1"
    } | Out-Null
    Write-Host ("    mock provider registered: " + $script:AiMockProviderID + " -> " + $BaseUrl)
    return $script:AiMockProviderID
}

function Unregister-AiMockProvider {
    param(
        [Parameter(Mandatory = $true)][string]$ApiBase,
        [Parameter(Mandatory = $true)][hashtable]$Headers
    )
    try {
        Invoke-AiApi -ApiBase $ApiBase -Method DELETE -Path ("/api/ai/providers/" + $script:AiMockProviderID) -Headers $Headers | Out-Null
        Write-Host ("    mock provider removed: " + $script:AiMockProviderID)
    } catch {
        if ($_.Exception.Message -notmatch "NOT_FOUND") {
            Write-Host ("    warn: remove mock provider failed: " + $_.Exception.Message)
        }
    }
}

function Assert-AiAnswerNotDegraded {
    param(
        $Result,
        [string]$Context = "chat"
    )
    if (-not $Result -or -not $Result.answer) {
        throw "$Context returned no answer"
    }
    if ([string]$Result.status -eq "degraded") {
        $reason = [string]$Result.answer.degraded_reason
        throw ("$Context is degraded (" + $reason + "). Ensure app.env=dev (loopback outbound) and the mock provider is enabled.")
    }
}
