$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root
New-Item -ItemType Directory -Force -Path (Join-Path $Root 'bin') | Out-Null
$Version = (Get-Content -LiteralPath (Join-Path $Root 'VERSION') -Raw).Trim()
go test ./...
go build -trimpath -buildvcs=true -ldflags "-s -w -X main.version=$Version" -o (Join-Path $Root 'bin\gia.exe') ./cmd/gia
Write-Output "Built: $Root\bin\gia.exe"
