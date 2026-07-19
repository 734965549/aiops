<#
.SYNOPSIS
    verify-prod-version.ps1 的单元测试（纯 PowerShell，无 Pester 依赖）。

.DESCRIPTION
    覆盖三条核心用例 + 补充边界用例，断言 verify-prod-version.ps1 的 exit code：
      - 纯 tag（v1.2.3）：FAIL，exit 1
      - 完整 tag（registry/repo:v1.2.3）：WARN，exit 0
      - digest（registry/repo@sha256:64hex）：PASS，exit 0
      - 补充：latest、空、无 tag、无仓库路径 均应 FAIL

    全部通过输出 "ALL TESTS PASSED" 并 exit 0；任意失败 exit 1。

.EXAMPLE
    .\scripts\tests\verify-prod-version.tests.ps1

.NOTES
    依赖：无外部依赖，兼容 Windows PowerShell 5.1+。
#>

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path))
$verifyScript = Join-Path $repoRoot (Join-Path 'scripts' 'verify-prod-version.ps1')

$script:failureCount = 0

function Invoke-Verify {
    param([string]$Image)
    & $verifyScript -Image $Image 2>&1 | Out-Null
    return $LASTEXITCODE
}

function Assert-ExitCode {
    param(
        [string]$CaseName,
        [string]$Image,
        [int]$Expected,
        [string]$ExpectedLabel
    )
    $actual = Invoke-Verify -Image $Image
    if ($actual -eq $Expected) {
        Write-Host ("    [PASS] " + $CaseName + " -> " + $ExpectedLabel + " (exit=" + $actual + ")") -ForegroundColor Green
    } else {
        Write-Host ("    [FAIL] " + $CaseName + " -> expected " + $ExpectedLabel + " (exit=" + $Expected + ") but got exit=" + $actual) -ForegroundColor Red
        $script:failureCount++
    }
}

Write-Host "==> running verify-prod-version.ps1 test cases"

# 三条核心用例
Assert-ExitCode "pure-tag v1.2.3" "v1.2.3" 1 "FAIL"
Assert-ExitCode "full-tag registry/repo:v1.2.3" "registry.example.com/aiops-api:v1.2.3" 0 "WARN/PASS"
Assert-ExitCode "digest registry/repo@sha256:64hex" "registry.example.com/aiops-api@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" 0 "PASS"

# 补充边界用例
Assert-ExitCode "latest-tag registry/repo:latest" "registry.example.com/aiops-api:latest" 1 "FAIL"
Assert-ExitCode "bare latest" "latest" 1 "FAIL"
Assert-ExitCode "no-tag registry/repo" "registry.example.com/aiops-api" 1 "FAIL"
Assert-ExitCode "no-slash aiops-api:v1.2.3" "aiops-api:v1.2.3" 1 "FAIL"

if ($script:failureCount -eq 0) {
    Write-Host ""
    Write-Host "ALL TESTS PASSED" -ForegroundColor Green
    exit 0
} else {
    Write-Host ""
    Write-Host ("TESTS FAILED (" + $script:failureCount + " case(s) failed)") -ForegroundColor Red
    exit 1
}
