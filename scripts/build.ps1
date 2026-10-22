$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    Push-Location (Join-Path $projectRoot 'frontend')
    try { npm.cmd ci; if ($LASTEXITCODE -ne 0) { throw 'Frontend install failed' }; npm.cmd run build; if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' } } finally { Pop-Location }
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    $goPath = go env GOPATH
    $wailsPath = Join-Path $goPath 'bin\wails.exe'
    if (-not (Test-Path -LiteralPath $wailsPath)) { go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2; if ($LASTEXITCODE -ne 0) { throw 'Wails install failed' } }
    & $wailsPath build -s
    if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }
    go build -o build/bin/NovelEdit-browser.exe ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw 'Browser edition build failed' }
} finally { Pop-Location }
