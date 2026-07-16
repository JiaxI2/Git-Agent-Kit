[CmdletBinding(SupportsShouldProcess)]
param()

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$TemporaryRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$Tmp = Join-Path $TemporaryRoot ("gia-test-" + [guid]::NewGuid())
$Remote = Join-Path $Tmp 'remote.git'
$Repo = Join-Path $Tmp 'repo'
$WtRoot = Join-Path $Tmp 'worktrees'

function Get-VerifiedTemporaryPath {
  param(
    [Parameter(Mandatory)]
    [string] $Path,

    [Parameter(Mandatory)]
    [string] $TemporaryRoot
  )

  $rootFull = [System.IO.Path]::GetFullPath($TemporaryRoot).TrimEnd(
    [System.IO.Path]::DirectorySeparatorChar,
    [System.IO.Path]::AltDirectorySeparatorChar
  )
  $pathFull = [System.IO.Path]::GetFullPath($Path)
  $prefix = $rootFull + [System.IO.Path]::DirectorySeparatorChar
  if (-not $pathFull.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Refusing cleanup outside temporary root '$rootFull': $pathFull"
  }
  return $pathFull
}

function Remove-GiaTestPath {
  [CmdletBinding(SupportsShouldProcess)]
  param(
    [Parameter(Mandatory)]
    [string] $Path,

    [Parameter(Mandatory)]
    [string] $TemporaryRoot,

    [switch] $Recurse
  )

  $safePath = Get-VerifiedTemporaryPath -Path $Path -TemporaryRoot $TemporaryRoot
  if (-not (Test-Path -LiteralPath $safePath)) {
    return
  }
  if (-not $PSCmdlet.ShouldProcess($safePath, 'Remove GIA temporary test path')) {
    return
  }
  Remove-Item -LiteralPath $safePath -Recurse:$Recurse -Force -Confirm:$false -ErrorAction Stop
}

if (-not $PSCmdlet.ShouldProcess($Tmp, 'Create and execute the isolated GIA local integration test')) {
  return
}

try {
  git init --bare $Remote | Out-Null
  git clone $Remote $Repo | Out-Null
  git -C $Repo config user.email gia-test@example.invalid
  git -C $Repo config user.name gia-test
  "module example.com/sample`n`ngo 1.22`n" | Set-Content -Encoding utf8 (Join-Path $Repo 'go.mod')
  "package sample`nimport `"testing`"`nfunc TestOK(t *testing.T) {}`n" | Set-Content -Encoding utf8 (Join-Path $Repo 'main_test.go')
  git -C $Repo add .
  git -C $Repo commit -m init | Out-Null
  git -C $Repo branch -M main
  git -C $Repo push -u origin main | Out-Null
  & "$Root\bin\gia.exe" init --repo $Repo | Out-Null
  git -C $Repo add .gia
  git -C $Repo commit -m 'add gia config' | Out-Null
  git -C $Repo push | Out-Null
  $BaseBranch = git -C $Repo branch --show-current
  git -C $Repo branch agent/web-agent/feat/1-test origin/main
  $Result = & "$Root\bin\gia.exe" worktree create --repo $Repo --ref agent/web-agent/feat/1-test --root $WtRoot | ConvertFrom-Json
  $Wt = $Result.data.path
  $Head = git -C $Wt rev-parse HEAD
  if ((git -C $Repo branch --show-current) -ne $BaseBranch) { throw 'stable worktree branch changed' }
  & "$Root\bin\gia.exe" validate --repo $Wt --profile smoke --expected-head deadbeef 2>$null | Out-Null
  if ($LASTEXITCODE -eq 0) { throw 'expected SHA mismatch failure' }
  & "$Root\bin\gia.exe" validate --repo $Wt --profile smoke --expected-head $Head | Out-Null
  'dirty' | Set-Content (Join-Path $Wt 'dirty.txt')
  & "$Root\bin\gia.exe" worktree remove --repo $Repo --path $Wt 2>$null | Out-Null
  if ($LASTEXITCODE -eq 0) { throw 'expected dirty worktree refusal' }
  Remove-GiaTestPath -Path (Join-Path $Wt 'dirty.txt') -TemporaryRoot $Tmp -WhatIf:$WhatIfPreference
  & "$Root\bin\gia.exe" worktree remove --repo $Repo --path $Wt | Out-Null
  Write-Output 'local integration test passed'
} finally {
  Remove-GiaTestPath -Path $Tmp -TemporaryRoot $TemporaryRoot -Recurse -WhatIf:$WhatIfPreference
}
