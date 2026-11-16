# Shared helpers for service scripts. Dot-source from other scripts.

function Get-RepoRoot {
    Split-Path -Parent $PSScriptRoot
}

function Import-PixivEnv {
    param(
        [string]$EnvFile = (Join-Path (Get-RepoRoot) 'config.env')
    )
    if (-not (Test-Path -LiteralPath $EnvFile)) {
        throw "Missing config file: $EnvFile. Copy config.env.example to config.env first."
    }
    Get-Content -LiteralPath $EnvFile -Encoding UTF8 | ForEach-Object {
        $line = $_.Trim()
        if ($line -eq '' -or $line.StartsWith('#')) { return }
        $i = $line.IndexOf('=')
        if ($i -lt 1) { return }
        $key = $line.Substring(0, $i).Trim()
        $val = $line.Substring($i + 1).Trim()
        if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
            $val = $val.Substring(1, $val.Length - 2)
        }
        Set-Item -Path "Env:$key" -Value $val
    }
}

function Get-PixivExe {
    $root = Get-RepoRoot
    $exe = Join-Path $root 'pixiv-downloader.exe'
    if (-not (Test-Path -LiteralPath $exe)) {
        throw "Binary not found: $exe. Run .\build.ps1 first."
    }
    return $exe
}

function Stop-PixivDownloader {
    Get-Process -Name 'pixiv-downloader' -ErrorAction SilentlyContinue | ForEach-Object {
        Stop-Process -Id $_.Id -Force -ErrorAction SilentlyContinue
    }
    Start-Sleep -Milliseconds 500
}