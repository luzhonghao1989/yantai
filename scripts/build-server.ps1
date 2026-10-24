param([switch]$SkipFrontend)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
$previousGoOs = $env:GOOS
$previousGoArch = $env:GOARCH
$previousCgo = $env:CGO_ENABLED
try {
    if (-not $SkipFrontend) {
        npm.cmd run build --prefix frontend
        if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
    }
    New-Item -ItemType Directory -Path build/bin/server-linux-amd64 -Force | Out-Null
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    go build -trimpath -o build/bin/Yantai-server.exe ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Windows Server build failed' }
    $env:GOOS = 'linux'
    go build -trimpath -o build/bin/server-linux-amd64/Yantai-server ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Linux server build failed' }
} finally {
    $env:GOOS = $previousGoOs
    $env:GOARCH = $previousGoArch
    $env:CGO_ENABLED = $previousCgo
    Pop-Location
}
