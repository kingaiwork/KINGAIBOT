param(
  [string]$Repo = $(if ($env:KINGAGENT_REPO) { $env:KINGAGENT_REPO } else { throw "KINGAGENT_REPO must be set" })
)
$ErrorActionPreference = "Stop"
if ($Repo -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw "Invalid GitHub repository identifier." }
$repoParts = $Repo.Split('/')
if ($repoParts[0] -in @('.', '..') -or $repoParts[1] -in @('.', '..')) { throw "Invalid GitHub repository path segment." }
$repoRegex = [regex]::Escape($Repo)
$workflowIdentityPattern = "^https://github\.com/$repoRegex/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$asset = "kingaibot_windows_$arch.zip"
$base = "https://github.com/$Repo/releases/latest/download"
$tmp = Join-Path $env:TEMP ("kingagent-update-" + [guid]::NewGuid().ToString("N"))
$root = Join-Path $env:ProgramData "KINGAgent"
$bin = Join-Path $root "bin"
$clientBin = Join-Path $root "client\bin"
$taskName = "KINGAIBOT"
New-Item -ItemType Directory -Force $tmp | Out-Null

function Stop-KINGConsole {
  try {
    Get-CimInstance Win32_Process -Filter "Name='kingconsole.exe'" -ErrorAction SilentlyContinue | ForEach-Object {
      $path = $_.ExecutablePath
      if ($path -and $path.StartsWith($root, [System.StringComparison]::OrdinalIgnoreCase)) {
        Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
      }
    }
  } catch {}
}

function Backup-And-Copy([string]$Source, [string]$Target) {
  $prev = "$Target.prev"
  if (Test-Path $Target) { Copy-Item $Target $prev -Force }
  Copy-Item $Source $Target -Force
}

function Restore-Previous([string]$Target) {
  $prev = "$Target.prev"
  if (Test-Path $prev) { Copy-Item $prev $Target -Force }
}

function Remove-Previous([string]$Target) {
  Remove-Item "$Target.prev" -Force -ErrorAction SilentlyContinue
}

try {
  Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset)
  Invoke-WebRequest "$base/SHA256SUMS" -OutFile (Join-Path $tmp "SHA256SUMS")
  $line = Get-Content (Join-Path $tmp "SHA256SUMS") | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } | Select-Object -First 1
  if (-not $line) { throw "Checksum entry not found" }
  $expected = ($line -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash (Join-Path $tmp $asset) -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($expected -ne $actual) { throw "SHA256 verification failed" }

  $cosign = Get-Command cosign.exe -ErrorAction SilentlyContinue
  if (-not $cosign) { $cosign = Get-Command cosign -ErrorAction SilentlyContinue }
  if ($cosign) {
    $bundle = Join-Path $tmp ($asset + ".sigstore.json")
    try {
      Invoke-WebRequest "$base/$asset.sigstore.json" -OutFile $bundle
      & $cosign.Source verify-blob --bundle $bundle --certificate-oidc-issuer "https://token.actions.githubusercontent.com" --certificate-identity-regexp $workflowIdentityPattern (Join-Path $tmp $asset) | Out-Null
      if ($LASTEXITCODE -ne 0) { throw "Sigstore identity verification failed" }
    } catch { throw "Sigstore verification failed: $($_.Exception.Message)" }
  } elseif ($env:KINGAGENT_ALLOW_CHECKSUM_ONLY -ne "1") {
    throw "cosign is required for unattended updates. Set KINGAGENT_ALLOW_CHECKSUM_ONLY=1 only if you explicitly accept checksum-only verification."
  } else {
    Write-Warning "Proceeding with checksum-only update verification by explicit policy override."
  }

  $pkg = Join-Path $tmp "pkg"
  Expand-Archive (Join-Path $tmp $asset) -DestinationPath $pkg -Force
  $required = @('kingagentd.exe','kingagent.exe','kingworker.exe','kingconsole.exe','kingdesktop.exe')
  foreach ($name in $required) {
    if (-not (Test-Path (Join-Path $pkg $name))) { throw "Release asset is missing $name" }
  }

  $newVersion = (& (Join-Path $pkg "kingagentd.exe") -version).Trim()
  $oldVersion = if (Test-Path (Join-Path $bin "kingagentd.exe")) { (& (Join-Path $bin "kingagentd.exe") -version).Trim() } else { "none" }
  if ($newVersion -eq $oldVersion) { exit 0 }
  if ($oldVersion -ne "none" -and $env:KINGAGENT_ALLOW_DOWNGRADE -ne "1") {
    $newCore = ($newVersion -split '[-+]')[0]
    $oldCore = ($oldVersion -split '[-+]')[0]
    $newParsed = $null
    $oldParsed = $null
    if (-not [version]::TryParse($newCore, [ref]$newParsed) -or -not [version]::TryParse($oldCore, [ref]$oldParsed)) {
      throw "Cannot safely compare versions ($oldVersion -> $newVersion); refusing unattended update."
    }
    if ($newParsed -le $oldParsed) { throw "Refusing downgrade or non-increasing version: $oldVersion -> $newVersion" }
  }

  $wasReady = $false
  try { Invoke-WebRequest "http://127.0.0.1:18888/readyz" -UseBasicParsing -TimeoutSec 5 | Out-Null; $wasReady = $true } catch {}

  Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  Stop-KINGConsole
  New-Item -ItemType Directory -Force $bin,$clientBin | Out-Null

  $runtimeTargets = @()
  foreach ($name in $required) {
    $target = Join-Path $bin $name
    Backup-And-Copy (Join-Path $pkg $name) $target
    $runtimeTargets += $target
  }
  $clientTargets = @()
  foreach ($name in @('kingconsole.exe','kingdesktop.exe')) {
    $target = Join-Path $clientBin $name
    Backup-And-Copy (Join-Path $pkg $name) $target
    $clientTargets += $target
  }

  Start-ScheduledTask -TaskName $taskName
  Start-Sleep -Seconds 3
  try {
    Invoke-WebRequest "http://127.0.0.1:18888/healthz" -UseBasicParsing -TimeoutSec 5 | Out-Null
    if ($wasReady) { Invoke-WebRequest "http://127.0.0.1:18888/readyz" -UseBasicParsing -TimeoutSec 5 | Out-Null }
  } catch {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Stop-KINGConsole
    foreach ($target in $runtimeTargets) { Restore-Previous $target }
    foreach ($target in $clientTargets) { Restore-Previous $target }
    Start-ScheduledTask -TaskName $taskName
    throw "Update failed post-update health/readiness continuity check; runtime and visual client rolled back."
  }

  foreach ($target in $runtimeTargets) { Remove-Previous $target }
  foreach ($target in $clientTargets) { Remove-Previous $target }
  Write-Host "Updated KINGAIBOT runtime + visual client: $oldVersion -> $newVersion"
} finally {
  Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
