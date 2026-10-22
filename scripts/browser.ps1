$projectRoot = Split-Path -Parent $PSScriptRoot
$exePath = Join-Path $projectRoot 'build\bin\Yantai-browser.exe'
$dataPath = Join-Path $projectRoot 'build\bin\data'
Start-Process -FilePath $exePath -ArgumentList @('-data', ('"' + $dataPath + '"'), '-listen', '127.0.0.1:48120') -WorkingDirectory $projectRoot -WindowStyle Hidden
Start-Process 'http://127.0.0.1:48120'
