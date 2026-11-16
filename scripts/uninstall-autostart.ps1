# Remove the Pixiv Shelf scheduled task.
param(
    [string]$TaskName = 'PixivShelf',
    [switch]$Stop
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
Write-Host "Scheduled task removed: $TaskName"

if ($Stop) {
    & (Join-Path $PSScriptRoot 'stop.ps1')
}