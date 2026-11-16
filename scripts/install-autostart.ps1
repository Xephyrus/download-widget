# Register a Windows Scheduled Task so Pixiv Shelf starts at user logon.
param(
    [string]$TaskName = 'PixivShelf',
    [switch]$StartNow
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$root = Get-RepoRoot
$startScript = Join-Path $PSScriptRoot 'start.ps1'
if (-not (Test-Path -LiteralPath $startScript)) {
    throw "Missing $startScript"
}
if (-not (Test-Path -LiteralPath (Join-Path $root 'config.env'))) {
    throw "Copy config.env.example to config.env and set DOWNLOAD_DIR etc. first."
}
$null = Get-PixivExe

$ps = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$arg = '-NoProfile -ExecutionPolicy Bypass -File "' + $startScript + '"'

$action = New-ScheduledTaskAction -Execute $ps -Argument $arg -WorkingDirectory $root
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Principal $principal `
    -Force | Out-Null

Write-Host "Scheduled task registered: $TaskName (starts at logon for $env:USERNAME)"
Write-Host "Repo: $root"

if ($StartNow) {
    & $startScript
}