$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root
New-Item -ItemType Directory -Force -Path (Join-Path $Root 'bin') | Out-Null
go test ./...
go build -o (Join-Path $Root 'bin\gia.exe') ./cmd/gia
Write-Host "Built: $Root\bin\gia.exe"
