# fkteams Windows 安装脚本
# 用法: powershell -c "irm https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.ps1 | iex"

param(
    [string]$InstallDir = "$env:USERPROFILE\fkteams"
)

$ErrorActionPreference = "Stop"
# 隐藏 Invoke-WebRequest 的进度条（显著提升下载速度）
$ProgressPreference = "SilentlyContinue"

$GITHUB_REPO = "wsshow/feikong-teams"
$APP_NAME    = "fkteams"

function Write-Info  { param([string]$Msg) Write-Host "==> $Msg" -ForegroundColor Cyan }
function Write-Ok    { param([string]$Msg) Write-Host "==> $Msg" -ForegroundColor Green }
function Write-Warn  { param([string]$Msg) Write-Host "警告: $Msg" -ForegroundColor Yellow }
function Write-Fail  { param([string]$Msg) Write-Host "错误: $Msg" -ForegroundColor Red; exit 1 }

# ---- 检测 CPU 架构 ----
function Get-Arch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64" }
        default { Write-Fail "不支持的 CPU 架构: $arch (仅支持 AMD64 和 ARM64)" }
    }
}

# ---- 获取最新版本号 ----
function Get-LatestVersion {
    $apiUrl = "https://api.github.com/repos/$GITHUB_REPO/releases/latest"
    try {
        $headers = @{ "Accept" = "application/vnd.github.v3+json"; "User-Agent" = "fkteams-installer" }
        $release = Invoke-RestMethod -Uri $apiUrl -Headers $headers -UseBasicParsing
        return $release.tag_name
    }
    catch {
        Write-Fail "无法获取最新版本信息，请检查网络连接: $_"
    }
}

# ---- 将目录添加到用户 PATH ----
function Add-ToUserPath {
    param([string]$Dir)

    $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
    # 拆分后逐条比较，避免部分路径误匹配
    $pathItems = $currentPath -split ";" | Where-Object { $_ -ne "" }

    if ($pathItems -contains $Dir) {
        Write-Info "$Dir 已在用户 PATH 中，无需修改"
        return
    }

    $newPath = ($pathItems + $Dir) -join ";"
    [System.Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    Write-Ok "已将 $Dir 添加到用户 PATH"
    Write-Warn "请重启终端使 PATH 变更生效"
}

# ======== 主流程 ========

Write-Info "正在安装 $APP_NAME..."

$arch    = Get-Arch
$tag     = Get-LatestVersion
$version = $tag.TrimStart("v")   # GoReleaser 打包时去掉了 v 前缀

$zipName     = "feikong-teams_${version}_Windows_${arch}.zip"
$downloadUrl = "https://github.com/$GITHUB_REPO/releases/download/$tag/$zipName"

Write-Info "版本   : $tag"
Write-Info "平台   : Windows/$arch"
Write-Info "安装目录: $InstallDir"

# 创建临时目录
$tmpDir  = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmpDir | Out-Null

try {
    $zipPath = Join-Path $tmpDir $zipName

    # ---- 下载 ----
    Write-Info "正在下载 $zipName..."
    Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing

    # ---- 下载 checksums.txt 并校验 ----
    $checksumsUrl  = "https://github.com/$GITHUB_REPO/releases/download/$tag/checksums.txt"
    $checksumsPath = Join-Path $tmpDir "checksums.txt"
    Write-Info "正在验证文件完整性..."
    Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing

    $expectedLine = Get-Content $checksumsPath | Where-Object { $_ -match "\s$([regex]::Escape($zipName))$" }
    if ($expectedLine) {
        $expectedHash = ($expectedLine -split "\s+")[0].ToUpper()
        $actualHash   = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash.ToUpper()
        if ($actualHash -ne $expectedHash) {
            Write-Fail "SHA256 校验失败！`n  期望: $expectedHash`n  实际: $actualHash`n文件可能已损坏，请重试"
        }
        Write-Ok "SHA256 校验通过"
    }
    else {
        Write-Warn "checksums.txt 中未找到 $zipName 的校验值，跳过校验"
    }

    # ---- 创建安装目录 ----
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    # ---- 解压（覆盖已有文件）----
    Write-Info "正在解压..."
    Expand-Archive -Path $zipPath -DestinationPath $InstallDir -Force

    $exePath = Join-Path $InstallDir "$APP_NAME.exe"
    if (-not (Test-Path $exePath)) {
        Write-Fail "解压后未找到 $APP_NAME.exe，请检查压缩包内容"
    }

    Write-Ok "$APP_NAME 已安装至 $exePath"

    # ---- 将安装目录加入用户 PATH ----
    Add-ToUserPath $InstallDir

    Write-Ok "安装完成！重启终端后运行 '$APP_NAME --version' 验证安装。"
}
finally {
    # 清理临时文件
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
