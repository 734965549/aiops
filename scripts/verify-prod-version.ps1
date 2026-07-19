<#
.SYNOPSIS
    生产发布前置校验：强制完整镜像引用，拒绝 latest / 空 / 纯 tag。

.DESCRIPTION
    docker-compose.prod.yml 的 image: ${AIOPS_IMAGE:?...} 只能保证 AIOPS_IMAGE 非空，
    无法拒绝 latest 等可漂移标签，也无法判断是否为合法完整引用。本脚本在执行
    docker compose -f deployments/docker-compose.prod.yml up 之前补这个缺口。

    AIOPS_IMAGE 必须是完整镜像引用（compose 不再拼接仓库名）：
      - 空：FAIL，exit 1。
      - 以 :latest 结尾或裸 latest（忽略大小写）：FAIL，exit 1。
      - 以 @sha256:<64 位十六进制> 结尾（digest）：PASS，exit 0。
      - 完整 tag（含仓库路径 + :tag，如 registry/repo:v1.2.0）：WARN，exit 0，建议改用 digest。
      - 其它（纯 tag 如 v1.2.0、无 tag 如 registry/repo、无仓库路径如 aiops-api:v1.2.0）：FAIL，exit 1。

.PARAMETER Image
    完整镜像引用。未传时读取环境变量 AIOPS_IMAGE。

.EXAMPLE
    $env:AIOPS_IMAGE = "registry.example.com/aiops-api@sha256:0123...def"
    .\scripts\verify-prod-version.ps1

.EXAMPLE
    .\scripts\verify-prod-version.ps1 -Image "registry.example.com/aiops-api:latest"
    # 输出 FAIL 并 exit 1

.NOTES
    依赖：无外部依赖，兼容 Windows PowerShell 5.1+。
    建议：生产优先使用镜像 digest（registry/repo@sha256:...），其次不可变版本号（registry/repo:v1.2.0）。
    可与 Makefile 的 docker-prod 目标配合：docker-prod 构建+push 后，以 digest 填入 AIOPS_IMAGE。
#>

param(
    [string]$Image = $env:AIOPS_IMAGE
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host ("==> " + $Message)
}

function Write-Pass {
    param([string]$Message)
    Write-Host ("    [PASS] " + $Message) -ForegroundColor Green
}

function Write-Fail {
    param([string]$Message)
    Write-Host ("    [FAIL] " + $Message) -ForegroundColor Red
}

Write-Step "validating AIOPS_IMAGE for production"

if ([string]::IsNullOrWhiteSpace($Image)) {
    Write-Fail "AIOPS_IMAGE 未设置或为空；生产必须使用完整镜像引用（registry/repo@sha256:... 或不可变 tag），禁止 latest。"
    exit 1
}

$normalized = $Image.Trim()

if ($normalized -ieq "latest" -or $normalized -imatch ':latest$') {
    Write-Fail ("AIOPS_IMAGE='" + $normalized + "' 被拒绝；生产禁止使用 latest，请使用 digest（registry/repo@sha256:...）或不可变 tag（registry/repo:v1.2.0）。")
    exit 1
}

if ($normalized -match '@sha256:[0-9a-fA-F]{64}$') {
    Write-Pass ("AIOPS_IMAGE 使用 digest，不可变： " + $normalized)
    exit 0
}

# 非 digest：必须是完整 tag 引用（含仓库路径 + :tag，且 tag 非 latest）。
# 纯 tag（如 v1.2.3）、无 tag（如 registry/repo）、无仓库路径（如 aiops-api:v1.2.3）均拒绝。
if ($normalized -match '/' -and $normalized -match ':[^:/@]+$') {
    Write-Host ("    [WARN] AIOPS_IMAGE='" + $normalized + "' 不是 digest（registry/repo@sha256:...）。生产强烈建议使用 digest 以保证不可变与可追溯；当前不阻塞，发布前请确认该 tag 不可变。") -ForegroundColor Yellow
    exit 0
}

Write-Fail ("AIOPS_IMAGE='" + $normalized + "' 不是合法完整镜像引用；必须是 digest（registry/repo@sha256:...）或完整 tag（registry/repo:v1.2.0），禁止纯 tag、裸 latest、无 tag 或无仓库路径。")
exit 1
