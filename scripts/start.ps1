# Start Pixiv Shelf in the background (loads config.env, writes logs/).
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$root = Get-RepoRoot
Set-Location $root
Import-PixivEnv
$exe = Get-PixivExe

$logDir = Join-Path $root 'logs'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$stdout = Join-Path $logDir 'service.out.log'
$stderr = Join-Path $logDir 'service.err.log'

Stop-PixivDownloader

$p = Start-Process -FilePath $exe `
    -WorkingDirectory $root `
    -WindowStyle Hidden `
    -RedirectStandardOutput $stdout `
    -RedirectStandardError $stderr `
    -PassThru

Start-Sleep -Seconds 1
if ($p.HasExited) {
    throw "Process exited immediately. Check logs/service.err.log"
}

Write-Host "Pixiv Shelf running in background (PID $($p.Id))"
Write-Host "URL: http://127.0.0.1:$($env:PORT)"
Write-Host "Logs: $stdout"