# fkteams Windows 安装脚本
# 用法: powershell -c "irm https://raw.githubusercontent.com/wsshow/feikong-teams/main/install.ps1 | iex"

param(
    [string]$InstallDir = $(if ($env:FKTEAMS_INSTALL_DIR) { $env:FKTEAMS_INSTALL_DIR } else { "$env:USERPROFILE\.fkteams\bin" })
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

# ---- 获取终端代理参数（供 Invoke-WebRequest / Invoke-RestMethod 使用）----
function Get-ProxyParams {
    $proxyUrl = if ($env:https_proxy) { $env:https_proxy }
                elseif ($env:HTTPS_PROXY) { $env:HTTPS_PROXY }
                elseif ($env:http_proxy) { $env:http_proxy }
                elseif ($env:HTTP_PROXY) { $env:HTTP_PROXY }
                else { $null }
    if ($proxyUrl) {
        return @{ Proxy = $proxyUrl; ProxyUseDefaultCredentials = $true }
    }
    return @{}
}

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
        $proxyParams = Get-ProxyParams
        $release = Invoke-RestMethod -Uri $apiUrl -Headers $headers -UseBasicParsing @proxyParams
        return $release.tag_name
    }
    catch {
        if ($_.Exception.Response.StatusCode -eq 403) {
            Write-Fail "GitHub API 速率限制，请稍后重试"
        }
        Write-Fail "无法获取最新版本信息，请检查网络连接: $_"
    }
}

# ---- 自定义进度条下载（支持断点续传 + 重试）----
function Invoke-DownloadWithProgress {
    param(
        [string]$Url,
        [string]$Dest,
        [int]$MaxRetries = 5,
        [int]$RetryDelay  = 3
    )

    Add-Type -AssemblyName System.Net.Http

    # 用于 Ctrl+C 取消网络请求
    $cts = [System.Threading.CancellationTokenSource]::new()
    $cancelHandler = [System.ConsoleCancelEventHandler]{
        param($sender, $e)
        $e.Cancel = $true  # 阻止立即退出，让 finally 块清理资源
        $cts.Cancel()
    }
    [Console]::add_CancelKeyPress($cancelHandler)

    try {
    for ($attempt = 1; $attempt -le $MaxRetries; $attempt++) {
        # 若文件已存在则尝试断点续传
        $resumeOffset = 0
        if (Test-Path $Dest) {
            $resumeOffset = (Get-Item $Dest).Length
        }

        try {
            $handler = [System.Net.Http.HttpClientHandler]::new()

            # 继承终端的 https_proxy / http_proxy 环境变量
            $proxyUrl = if ($env:https_proxy) { $env:https_proxy }
                        elseif ($env:HTTPS_PROXY) { $env:HTTPS_PROXY }
                        elseif ($env:http_proxy) { $env:http_proxy }
                        elseif ($env:HTTP_PROXY) { $env:HTTP_PROXY }
                        else { $null }
            if ($proxyUrl) {
                $handler.Proxy = [System.Net.WebProxy]::new($proxyUrl)
                $handler.UseProxy = $true
            }

            $client  = [System.Net.Http.HttpClient]::new($handler)
            $client.DefaultRequestHeaders.UserAgent.ParseAdd("fkteams-installer/1.0")
            $client.Timeout = [System.TimeSpan]::FromMinutes(30)

            # 设置 Range 头以支持断点续传
            if ($resumeOffset -gt 0) {
                $client.DefaultRequestHeaders.Range = [System.Net.Http.Headers.RangeHeaderValue]::new($resumeOffset, $null)
            }

            $response = $client.GetAsync(
                $Url,
                [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead,
                $cts.Token
            ).GetAwaiter().GetResult()

            # 200: 服务器不支持 Range，重头下载；206: 断点续传
            $isResume = ($response.StatusCode -eq [System.Net.HttpStatusCode]::PartialContent)
            if (-not $isResume) {
                $response.EnsureSuccessStatusCode() | Out-Null
                $resumeOffset = 0  # 服务器返回 200，忽略已有文件
            }

            # 计算总大小
            $totalBytes = [long]0
            $cr = $response.Content.Headers.ContentRange
            if ($null -ne $cr -and $cr.HasLength) {
                $totalBytes = $cr.Length  # Content-Range 提供准确总大小
            } elseif ($response.Content.Headers.ContentLength -gt 0) {
                $totalBytes = $response.Content.Headers.ContentLength + $resumeOffset
            }

            $inStream  = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
            $fileMode  = if ($isResume) { [System.IO.FileMode]::Append } else { [System.IO.FileMode]::Create }
            $outStream = [System.IO.FileStream]::new($Dest, $fileMode, [System.IO.FileAccess]::Write)
            $buffer    = [byte[]]::new(65536)
            $downloaded = [long]$resumeOffset
            $lastPct    = -1
            $barWidth   = 40

            try {
                while (-not $cts.IsCancellationRequested) {
                    $readTask = $inStream.ReadAsync($buffer, 0, $buffer.Length, $cts.Token)
                    $bytesRead = $readTask.GetAwaiter().GetResult()
                    if ($bytesRead -eq 0) { break }

                    $outStream.Write($buffer, 0, $bytesRead)
                    $downloaded += $bytesRead

                    if ($totalBytes -gt 0) {
                        $pct = [int]($downloaded * 100 / $totalBytes)
                        if ($pct -ne $lastPct) {
                            $lastPct = $pct
                            $filled  = [int]($pct * $barWidth / 100)
                            $empty   = $barWidth - $filled
                            $bar     = ('#' * $filled) + ('-' * $empty)
                            $dlMB    = [math]::Round($downloaded / 1MB, 1)
                            $totMB   = [math]::Round($totalBytes  / 1MB, 1)
                            Write-Host ("`r  [$bar] {0,3}%  $dlMB MB / $totMB MB" -f $pct) -NoNewline
                        }
                    }
                }
                if ($cts.IsCancellationRequested) {
                    Write-Host ""
                    Write-Warn "下载已被用户取消"
                    exit 1
                }
                Write-Host ""
                return
            }
            finally {
                $outStream.Dispose()
                $inStream.Dispose()
            }
        }
        catch [System.OperationCanceledException] {
            Write-Host ""
            Write-Warn "下载已被用户取消"
            exit 1
        }
        catch {
            if ($attempt -lt $MaxRetries) {
                Write-Warn "下载出错（第 ${attempt}/${MaxRetries} 次），${RetryDelay}s 后重试..."
                Start-Sleep -Seconds $RetryDelay
                if ($cts.IsCancellationRequested) {
                    Write-Host ""
                    Write-Warn "下载已被用户取消"
                    exit 1
                }
            }
            else {
                Write-Fail "下载失败（已重试 $MaxRetries 次）: $_"
            }
        }
        finally {
            if ($null -ne $client) { $client.Dispose() }
        }
    }
    }
    finally {
        [Console]::remove_CancelKeyPress($cancelHandler)
        $cts.Dispose()
    }
}

# ---- 将目录添加到用户 PATH ----
function Add-ToUserPath {
    param([string]$Dir)

    $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")
    # 拆分后逐条 trim 并过滤空项，避免路径粘连
    $pathItems = $currentPath -split ";" | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne "" }

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
    Invoke-DownloadWithProgress -Url $downloadUrl -Dest $zipPath

    # ---- 下载 checksums.txt 并校验 ----
    $checksumsUrl  = "https://github.com/$GITHUB_REPO/releases/download/$tag/checksums.txt"
    $checksumsPath = Join-Path $tmpDir "checksums.txt"
    Write-Info "正在验证文件完整性..."
    $proxyParams = Get-ProxyParams
    Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing @proxyParams

    $expectedLine = Get-Content $checksumsPath | Where-Object { $_ -match "^[0-9a-fA-F]{64}\s+$([regex]::Escape($zipName))$" }
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
