param(
  [string]$Repo = $(if ($env:KINGAGENT_REPO) { $env:KINGAGENT_REPO } else { "kingaiwork/KINGAIBOT" })
)
$ErrorActionPreference = "Stop"
if ($Repo -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw "Invalid GitHub repository identifier." }
$repoParts = $Repo.Split('/')
if ($repoParts[0] -in @('.', '..') -or $repoParts[1] -in @('.', '..')) { throw "Invalid GitHub repository path segment." }
$repoRegex = [regex]::Escape($Repo)
$workflowIdentityPattern = "^https://github\.com/$repoRegex/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$"
function Ensure-RandomTokenFile([string]$Path) {
  if (-not (Test-Path $Path)) {
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    $tokenText = -join ($bytes | ForEach-Object { $_.ToString("x2") })
    [IO.File]::WriteAllText($Path, $tokenText, (New-Object Text.UTF8Encoding($false)))
  }
}
function New-KINGShortcut([string]$Path, [string]$Target, [string]$WorkingDirectory) {
  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut($Path)
  $shortcut.TargetPath = $Target
  $shortcut.WorkingDirectory = $WorkingDirectory
  $shortcut.Description = "KING AI Control Center"
  $shortcut.Save()
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw "Run PowerShell as Administrator." }
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$asset = "kingaibot_windows_$arch.zip"
$channel = if ($env:KINGAGENT_CHANNEL) { $env:KINGAGENT_CHANNEL } else { "latest" }
if ($channel -eq "latest") { $base = "https://github.com/$Repo/releases/latest/download" }
elseif ($channel -match '^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9._-]+)?$') { $base = "https://github.com/$Repo/releases/download/$channel" }
else { throw "KINGAGENT_CHANNEL must be 'latest' or a vMAJOR.MINOR.PATCH release tag." }
$tmp = Join-Path $env:TEMP ("kingagent-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force $tmp | Out-Null
try {
  Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $tmp $asset)
  Invoke-WebRequest "$base/SHA256SUMS" -OutFile (Join-Path $tmp "SHA256SUMS")
  $line = (Get-Content (Join-Path $tmp "SHA256SUMS") | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } | Select-Object -First 1)
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
  } elseif ($env:KINGAGENT_REQUIRE_SIGNATURE -eq "1") { throw "KINGAGENT_REQUIRE_SIGNATURE=1 but cosign is not installed." }
  else { Write-Warning "cosign not found; initial install is checksum-verified only. Set KINGAGENT_REQUIRE_SIGNATURE=1 to fail closed." }

  $pkg = Join-Path $tmp "pkg"
  Expand-Archive (Join-Path $tmp $asset) -DestinationPath $pkg -Force
  foreach ($required in @('kingagentd.exe','kingagent.exe','kingworker.exe','kingconsole.exe','kingdesktop.exe')) {
    if (-not (Test-Path (Join-Path $pkg $required))) { throw "Release asset is missing $required" }
  }

  $root = Join-Path $env:ProgramData "KINGAgent"
  $bin = Join-Path $root "bin"
  $data = Join-Path $root "data"
  $workspace = Join-Path $data "workspace"
  $clientRoot = Join-Path $root "client"
  $clientBin = Join-Path $clientRoot "bin"
  $providerSecrets = Join-Path $root "provider-secrets"
  $taskName = "KINGAIBOT"
  $updateTask = "KINGAIBOT Update"
  schtasks.exe /End /TN $taskName 2>$null | Out-Null
  schtasks.exe /Delete /TN $taskName /F 2>$null | Out-Null
  schtasks.exe /End /TN $updateTask 2>$null | Out-Null
  schtasks.exe /Delete /TN $updateTask /F 2>$null | Out-Null
  New-Item -ItemType Directory -Force $bin,$workspace,$clientBin,$providerSecrets | Out-Null

  Copy-Item (Join-Path $pkg "kingagentd.exe") (Join-Path $bin "kingagentd.exe") -Force
  Copy-Item (Join-Path $pkg "kingagent.exe") (Join-Path $bin "kingagent.exe") -Force
  Copy-Item (Join-Path $pkg "kingworker.exe") (Join-Path $bin "kingworker.exe") -Force
  Copy-Item (Join-Path $pkg "kingconsole.exe") (Join-Path $bin "kingconsole.exe") -Force
  Copy-Item (Join-Path $pkg "kingdesktop.exe") (Join-Path $bin "kingdesktop.exe") -Force
  Copy-Item (Join-Path $pkg "kingconsole.exe") (Join-Path $clientBin "kingconsole.exe") -Force
  Copy-Item (Join-Path $pkg "kingdesktop.exe") (Join-Path $clientBin "kingdesktop.exe") -Force
  Copy-Item (Join-Path $pkg "update.ps1") (Join-Path $root "update.ps1") -Force
  if (Test-Path (Join-Path $pkg "providers.catalog.json")) { Copy-Item (Join-Path $pkg "providers.catalog.json") (Join-Path $root "providers.catalog.json") -Force }
  if (Test-Path (Join-Path $pkg "COGNITIVE-RUNTIME.md")) { Copy-Item (Join-Path $pkg "COGNITIVE-RUNTIME.md") (Join-Path $root "COGNITIVE-RUNTIME.md") -Force }

  $config = Join-Path $root "config.json"
  if (-not (Test-Path $config)) {
    Copy-Item (Join-Path $pkg "config.example.json") $config
    $j = Get-Content $config -Raw | ConvertFrom-Json
    $j.runtime.data_dir = ($data -replace '\\','/')
    $j.runtime.workspace_dir = ($workspace -replace '\\','/')
    $j.security.file_read_roots = @(($workspace -replace '\\','/'))
    $j.security.file_write_roots = @(($workspace -replace '\\','/'))
    $jsonText = $j | ConvertTo-Json -Depth 20
    [IO.File]::WriteAllText($config, $jsonText, (New-Object Text.UTF8Encoding($false)))
  }

  $adminTokenFile = Join-Path $root "admin-token.txt"
  $mcpTokenFile = Join-Path $root "mcp-token.txt"
  $a2aTokenFile = Join-Path $root "a2a-token.txt"
  Ensure-RandomTokenFile $adminTokenFile
  Ensure-RandomTokenFile $mcpTokenFile
  Ensure-RandomTokenFile $a2aTokenFile
  [Environment]::SetEnvironmentVariable("KINGAGENT_REPO",$Repo,"Machine")

  # Provider secrets are extensible: any NAME.txt matching a safe environment
  # variable name is injected into the low-privilege runtime process only.
  $legacyModelKeyFile = Join-Path $root "model-api-key.txt"
  $openAISecret = Join-Path $providerSecrets "OPENAI_API_KEY.txt"
  if (-not (Test-Path $openAISecret)) {
    $existingModelKey = [Environment]::GetEnvironmentVariable('OPENAI_API_KEY','Machine')
    if ((-not $existingModelKey) -and (Test-Path $legacyModelKeyFile)) { $existingModelKey = (Get-Content $legacyModelKeyFile -Raw).Trim() }
    [IO.File]::WriteAllText($openAISecret, $(if ($existingModelKey) { $existingModelKey } else { "" }), (New-Object Text.UTF8Encoding($false)))
    if ($existingModelKey) { [Environment]::SetEnvironmentVariable('OPENAI_API_KEY',$null,'Machine') }
  }
  foreach ($envName in @('ANTHROPIC_API_KEY','GEMINI_API_KEY','OPENROUTER_API_KEY','GROQ_API_KEY','COMPATIBLE_MODEL_API_KEY')) {
    $secretPath = Join-Path $providerSecrets ($envName + '.txt')
    if (-not (Test-Path $secretPath)) { [IO.File]::WriteAllText($secretPath, "", (New-Object Text.UTF8Encoding($false))) }
  }

  & icacls.exe $root /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" "*S-1-5-19:(OI)(CI)RX" | Out-Null
  & icacls.exe $data /grant:r "*S-1-5-19:(OI)(CI)M" | Out-Null
  & icacls.exe $clientRoot /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" "*S-1-5-32-545:(OI)(CI)RX" | Out-Null
  & icacls.exe $providerSecrets /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" "*S-1-5-19:(OI)(CI)RX" | Out-Null
  foreach ($tokenFile in @($adminTokenFile,$mcpTokenFile,$a2aTokenFile)) {
    & icacls.exe $tokenFile /inheritance:r /grant:r "*S-1-5-18:F" "*S-1-5-32-544:F" "*S-1-5-19:R" | Out-Null
  }
  Get-ChildItem -LiteralPath $providerSecrets -Filter '*.txt' -File | ForEach-Object {
    & icacls.exe $_.FullName /inheritance:r /grant:r "*S-1-5-18:F" "*S-1-5-32-544:F" "*S-1-5-19:R" | Out-Null
  }

  $runScript = Join-Path $root "run.ps1"
  @"
`$ErrorActionPreference = 'Stop'
`$env:KINGAGENT_ADMIN_TOKEN = (Get-Content '$adminTokenFile' -Raw).Trim()
`$env:KINGAGENT_MCP_TOKEN = (Get-Content '$mcpTokenFile' -Raw).Trim()
`$env:KINGAGENT_A2A_TOKEN = (Get-Content '$a2aTokenFile' -Raw).Trim()
`$env:KINGAGENT_REPO = '$Repo'
Get-ChildItem -LiteralPath '$providerSecrets' -Filter '*.txt' -File | ForEach-Object {
  `$envName = [IO.Path]::GetFileNameWithoutExtension(`$_.Name)
  if (`$envName -match '^[A-Z][A-Z0-9_]{1,127}$') {
    `$secretValue = (Get-Content -LiteralPath `$_.FullName -Raw).Trim()
    if (`$secretValue) { [Environment]::SetEnvironmentVariable(`$envName, `$secretValue, 'Process') }
  }
}
& '$bin\kingagentd.exe' -config '$config'
exit `$LASTEXITCODE
"@ | Set-Content -Encoding UTF8 $runScript
  & icacls.exe $runScript /inheritance:r /grant:r "*S-1-5-18:F" "*S-1-5-32-544:F" "*S-1-5-19:RX" | Out-Null

  $taskAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$runScript`""
  $taskTrigger = New-ScheduledTaskTrigger -AtStartup
  $taskPrincipal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\LOCAL SERVICE" -LogonType ServiceAccount -RunLevel Limited
  $taskSettings = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Days 3650)
  Register-ScheduledTask -TaskName $taskName -Action $taskAction -Trigger $taskTrigger -Principal $taskPrincipal -Settings $taskSettings -Force | Out-Null
  Start-ScheduledTask -TaskName $taskName

  $updateCmd = "powershell.exe -NoProfile -ExecutionPolicy Bypass -File `"$root\update.ps1`" -Repo `"$Repo`""
  schtasks.exe /Create /TN $updateTask /SC HOURLY /MO 6 /RU SYSTEM /RL HIGHEST /TR $updateCmd /F | Out-Null

  $desktop = [Environment]::GetFolderPath('Desktop')
  $startMenu = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs"
  New-Item -ItemType Directory -Force $startMenu | Out-Null
  New-KINGShortcut (Join-Path $desktop "KING AI Control Center.lnk") (Join-Path $clientBin "kingdesktop.exe") $clientBin
  New-KINGShortcut (Join-Path $startMenu "KING AI Control Center.lnk") (Join-Path $clientBin "kingdesktop.exe") $clientBin

  Write-Host "KINGAIBOT installed to $root"
  Write-Host "Runtime identity: NT AUTHORITY\LOCAL SERVICE (not Administrator/SYSTEM)."
  Write-Host "Signed updater identity: SYSTEM."
  Write-Host "KING AI Control Center installed machine-wide under $clientRoot and added to Desktop + Start Menu."
  Write-Host "Visual client URL: http://127.0.0.1:18889/ui/"
  Write-Host "Provider catalog: $root\providers.catalog.json"
  Write-Host "API keys: create/edit ENVIRONMENT_VARIABLE.txt files under $providerSecrets, then restart the KINGAIBOT scheduled task."
} finally {
  Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
