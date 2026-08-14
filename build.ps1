$ErrorActionPreference = 'Stop'

# Use existing GOROOT if set; otherwise rely on go on PATH.
if ($env:GOROOT) {
  $env:Path = "$env:GOROOT/bin;" + $env:Path
}

Set-Location $PSScriptRoot

go mod tidy
go build -trimpath -ldflags="-s -w" -o pixiv-downloader.exe .

$size = [math]::Round((Get-Item pixiv-downloader.exe).Length / 1MB, 2)
Write-Host "Built pixiv-downloader.exe ($size MB)"
