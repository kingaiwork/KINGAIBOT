param()
$ErrorActionPreference = 'Stop'

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as Administrator.' }

$root = Join-Path $env:ProgramData 'KINGAgent'
$secretDir = Join-Path $root 'provider-secrets'
$stateFile = Join-Path $root 'data\cloud\state.json'
if (-not (Test-Path (Join-Path $root 'run.ps1'))) { throw 'KINGAIBOT installation not found. Install the Runtime first.' }
New-Item -ItemType Directory -Force $secretDir | Out-Null

function Write-ProtectedEnv([string]$Name, [string]$Value) {
  if ($Name -notmatch '^[A-Z][A-Z0-9_]{1,127}$') { throw "Invalid environment variable name: $Name" }
  $path = Join-Path $secretDir ($Name + '.txt')
  [IO.File]::WriteAllText($path, $Value, (New-Object Text.UTF8Encoding($false)))
  & icacls.exe $path /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' '*S-1-5-19:R' | Out-Null
}

function New-SyncKey {
  $bytes = New-Object byte[] 32
  $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
  try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
  return [Convert]::ToBase64String($bytes)
}

$token = $env:KINGAI_ENROLLMENT_TOKEN
if (-not $token) {
  $secure = Read-Host 'KING AI one-time enrollment token' -AsSecureString
  $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
  try { $token = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr) }
  finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr) }
}
if ($token -notlike 'kop_enroll_*') { throw 'Enrollment token must use the kop_enroll_ prefix.' }

$baseUrl = if ($env:KINGAI_CLOUD_BASE_URL) { $env:KINGAI_CLOUD_BASE_URL } else { 'https://api.kingai.work' }
if ($baseUrl -notmatch '^https://') { throw 'KINGAI_CLOUD_BASE_URL must use HTTPS.' }
$memorySync = if ($env:KINGAI_MEMORY_SYNC) { $env:KINGAI_MEMORY_SYNC } else { '0' }
$syncKey = $env:KINGAI_SYNC_KEY
if ($memorySync -eq '1' -and -not $syncKey) { $syncKey = New-SyncKey }

$values = [ordered]@{
  KINGAI_CLOUD_ENABLED = '1'
  KINGAI_CLOUD_BASE_URL = $baseUrl
  KINGAI_ENROLLMENT_TOKEN = $token
  KINGAI_CLOUD_ENVIRONMENT = $(if ($env:KINGAI_CLOUD_ENVIRONMENT) { $env:KINGAI_CLOUD_ENVIRONMENT } else { 'production' })
  KINGAI_NODE_CLASS = $(if ($env:KINGAI_NODE_CLASS) { $env:KINGAI_NODE_CLASS } else { 'server' })
  KINGAI_NODE_PROVIDER = $(if ($env:KINGAI_NODE_PROVIDER) { $env:KINGAI_NODE_PROVIDER } else { '' })
  KINGAI_NODE_REGION = $(if ($env:KINGAI_NODE_REGION) { $env:KINGAI_NODE_REGION } else { '' })
  KINGAI_CLOUD_HEARTBEAT_SECONDS = $(if ($env:KINGAI_CLOUD_HEARTBEAT_SECONDS) { $env:KINGAI_CLOUD_HEARTBEAT_SECONDS } else { '60' })
  KINGAI_CLOUD_REQUIRE_POLICY = $(if ($env:KINGAI_CLOUD_REQUIRE_POLICY) { $env:KINGAI_CLOUD_REQUIRE_POLICY } else { '0' })
  KINGAI_MEMORY_SYNC = $memorySync
  KINGAI_MEMORY_SYNC_SECONDS = $(if ($env:KINGAI_MEMORY_SYNC_SECONDS) { $env:KINGAI_MEMORY_SYNC_SECONDS } else { '900' })
  KINGAI_SYNC_KEY = $(if ($syncKey) { $syncKey } else { '' })
  KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT = $(if ($env:KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT) { $env:KINGAI_CLOUD_ALLOW_CUSTOM_ENDPOINT } else { '0' })
}
foreach ($entry in $values.GetEnumerator()) { Write-ProtectedEnv $entry.Key ([string]$entry.Value) }

$task = Get-ScheduledTask -TaskName 'KINGAIBOT' -ErrorAction Stop
Stop-ScheduledTask -TaskName $task.TaskName -ErrorAction SilentlyContinue
Start-ScheduledTask -TaskName $task.TaskName

for ($i = 0; $i -lt 20; $i++) {
  if (Test-Path $stateFile) {
    try {
      $state = Get-Content $stateFile -Raw | ConvertFrom-Json
      if ($state.enrolled -eq $true) {
        Write-ProtectedEnv 'KINGAI_ENROLLMENT_TOKEN' ''
        Write-Host 'KING AI Cloud enrollment complete. One-time token removed from protected runtime secrets.'
        Write-Host 'Cloud & Fleet: http://127.0.0.1:18889/ui/cloud/'
        exit 0
      }
    } catch {}
  }
  Start-Sleep -Milliseconds 500
}

Write-Warning 'Cloud enrollment was configured, but a durable enrolled state was not observed yet. Inspect the local Cloud & Fleet page or Scheduled Task logs before replacing the one-time token.'
exit 3
