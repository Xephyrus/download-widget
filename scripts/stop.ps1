# Stop background Pixiv Shelf process.
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$before = @(Get-Process -Name 'pixiv-downloader' -ErrorAction SilentlyContinue)
if ($before.Count -eq 0) {
    Write-Host 'No running pixiv-downloader process.'
    exit 0
}
Stop-PixivDownloader
$after = @(Get-Process -Name 'pixiv-downloader' -ErrorAction SilentlyContinue)
if ($after.Count -gt 0) {
    Write-Host "Failed to stop PID(s): $($after.Id -join ', ') (try Task Manager / admin)."
    exit 1
}
Write-Host "Stopped: $($before.Id -join ', ')"