# Run both development servers; stop both when either exits or Ctrl+C is pressed.
[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$ApiPort = 8080,
    [ValidateRange(1, 65535)]
    [int]$WebPort = 3000,
    [Alias('Routes')]
    [switch]$ListRoutes,
    [switch]$SkipDatabase,
    # Run the API with plain `go run` even when Air (hot reload) is available.
    [switch]$NoReload
)

$ErrorActionPreference = 'Stop'

foreach ($command in @('go')) {
    if (-not (Get-Command $command -CommandType Application -ErrorAction SilentlyContinue)) {
        throw "Required command '$command' is not installed or is not available on PATH."
    }
}

$apiDirectory = Join-Path -Path $PSScriptRoot -ChildPath 'api'
$webDirectory = Join-Path -Path $PSScriptRoot -ChildPath 'web'

function Show-WebRoutes {
    param([int]$Port)

    $appDirectory = Join-Path $webDirectory 'src\app'
    $pages = Get-ChildItem -LiteralPath $appDirectory -Recurse -File |
        Where-Object { $_.Name -match '^page\.(tsx|ts|jsx|js)$' }
    $paths = foreach ($page in $pages) {
        $relative = $page.DirectoryName.Substring($appDirectory.Length).TrimStart([char[]]'\/')
        $segments = @($relative -split '[\\/]' | Where-Object { $_ -and $_ -notmatch '^\(' -and $_ -notmatch '^@' })
        if (@($segments | Where-Object { $_ -match '^_' }).Count -gt 0) { continue }
        '/' + ($segments -join '/')
    }

    Write-Host ''
    Write-Host 'PayCore Web Pages (Next.js routes; not backend API endpoints)'
    Write-Host ('{0,-8} {1,-25} {2}' -f 'METHOD', 'PATH', 'URL')
    foreach ($path in @($paths | Sort-Object -Unique)) {
        Write-Host ('{0,-8} {1,-25} {2}' -f 'GET', $path, "http://localhost:$Port$path")
    }
    Write-Host ''
}

if ($ListRoutes) {
    Write-Host 'Listing registered routes only. No servers will be started.'
    Write-Host 'URLs use the requested ports; availability is not checked in list-only mode.'
    $routeExitCode = 1
    $previousPort = $env:PORT
    Push-Location -LiteralPath $apiDirectory
    try {
        $env:PORT = [string]$ApiPort
        # Native stderr includes normal Go logs in Windows PowerShell.
        $ErrorActionPreference = 'Continue'
        & go run ./cmd/server -routes
        $routeExitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = 'Stop'
        $env:PORT = $previousPort
        Pop-Location
    }
    if ($routeExitCode -ne 0) { exit $routeExitCode }
    Show-WebRoutes -Port $WebPort
    exit 0
}

foreach ($command in @('node', 'npm.cmd')) {
    if (-not (Get-Command $command -CommandType Application -ErrorAction SilentlyContinue)) {
        throw "Required command '$command' is not installed or is not available on PATH."
    }
}

function Find-DevelopmentPort {
    param([int]$PreferredPort, [string]$Service)

    for ($candidate = $PreferredPort; $candidate -le [Math]::Min(65535, $PreferredPort + 100); $candidate++) {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::IPv6Any, $candidate)
        $listener.Server.DualMode = $true
        $listener.Server.ExclusiveAddressUse = $true
        try {
            $listener.Start()
            if ($candidate -ne $PreferredPort) {
                Write-Host "$Service port $PreferredPort is unavailable; using $candidate."
            }
            return $candidate
        }
        catch [System.Net.Sockets.SocketException] {
            # Leave existing services alone and try the next development port.
        }
        finally {
            $listener.Stop()
        }
    }
    throw "No available $Service port found starting at $PreferredPort."
}

# Next.js allows one dev server per project and records it in .next\dev\lock.
# A server left over from an earlier run (e.g. after a closed terminal) would make
# the new web job exit immediately, so stop it if it is this project's Next server.
function Stop-StaleWebDevServer {
    $lockFile = Join-Path $webDirectory '.next\dev\lock'
    if (-not (Test-Path -LiteralPath $lockFile -PathType Leaf)) { return }
    try {
        $lock = Get-Content -LiteralPath $lockFile -Raw | ConvertFrom-Json
    }
    catch {
        return
    }
    if (-not $lock.pid) { return }
    $process = Get-CimInstance Win32_Process -Filter "ProcessId=$([int]$lock.pid)" -ErrorAction SilentlyContinue
    $expected = Join-Path $webDirectory 'node_modules\next'
    if ($null -eq $process -or $process.Name -ne 'node.exe' -or
        $process.CommandLine -notlike "*$expected*") {
        return
    }
    Write-Host "Stopping previous Next.js dev server for this project (PID $($lock.pid), $($lock.appUrl))."
    & taskkill.exe /PID ([int]$lock.pid) /T /F 2>$null | Out-Null
    Start-Sleep -Milliseconds 500
}
Stop-StaleWebDevServer

$ApiPort = Find-DevelopmentPort -PreferredPort $ApiPort -Service 'API'
$WebPort = Find-DevelopmentPort -PreferredPort $WebPort -Service 'Web'
if ($WebPort -eq $ApiPort) {
    if ($WebPort -eq 65535) { throw 'API and web must use different ports.' }
    $WebPort = Find-DevelopmentPort -PreferredPort ($WebPort + 1) -Service 'Web'
}

$nextLauncher = Join-Path $webDirectory 'node_modules\.bin\next.cmd'
$nextPackage = Join-Path $webDirectory 'node_modules\next\package.json'
if (-not (Test-Path -LiteralPath $nextLauncher -PathType Leaf) -or
    -not (Test-Path -LiteralPath $nextPackage -PathType Leaf)) {
    Write-Host 'Web dependencies are missing or incomplete. Installing npm dependencies...'
    Push-Location -LiteralPath $webDirectory
    try {
        $ErrorActionPreference = 'Continue'
        & npm.cmd install --no-audit --no-fund
        if ($LASTEXITCODE -ne 0) {
            throw "npm install failed with exit code $LASTEXITCODE."
        }
    }
    finally {
        $ErrorActionPreference = 'Stop'
        Pop-Location
    }
    if (-not (Test-Path -LiteralPath $nextLauncher -PathType Leaf) -or
        -not (Test-Path -LiteralPath $nextPackage -PathType Leaf)) {
        throw 'Next.js is still incomplete after npm install. Check the npm installation output.'
    }
}

# An explicit database URL opts out of the bundled local database automatically.
$hasCustomDatabase = -not [string]::IsNullOrWhiteSpace($env:DATABASE_URL)
$apiEnvFile = Join-Path $apiDirectory '.env'
if ((Test-Path -LiteralPath $apiEnvFile) -and
    (Select-String -LiteralPath $apiEnvFile -Pattern '^\s*(export\s+)?DATABASE_URL\s*=' -Quiet)) {
    $hasCustomDatabase = $true
}
if (-not $SkipDatabase -and -not $hasCustomDatabase) {
    if (-not (Get-Command docker -CommandType Application -ErrorAction SilentlyContinue)) {
        throw 'Docker is required for local PostgreSQL. Start Docker Desktop, or configure DATABASE_URL and use -SkipDatabase.'
    }
    Write-Host 'Starting PayCore PostgreSQL at 127.0.0.1:5434...'
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & docker compose -f (Join-Path $PSScriptRoot 'compose.yaml') up -d --wait postgres
        if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL startup failed. Check Docker Desktop and port 5434.' }
    }
    finally {
        $ErrorActionPreference = $previousErrorAction
    }
}
else {
    Write-Host 'Automatic database startup skipped; the API will verify its configured DATABASE_URL.'
}

# Local email inbox (Mailpit) for invitation emails, unless SMTP is configured.
function Get-DotEnvValue {
    param([string]$Path, [string]$Name)
    if (-not (Test-Path -LiteralPath $Path)) { return '' }
    foreach ($line in Get-Content -LiteralPath $Path) {
        if ($line -match ('^\s*(?:export\s+)?' + [regex]::Escape($Name) + '\s*=\s*(.*)$')) {
            $value = $Matches[1].Trim()
            if ($value.StartsWith('"') -or $value.StartsWith("'")) {
                $quote = $value.Substring(0, 1)
                $end = $value.IndexOf($quote, 1)
                if ($end -gt 0) { return $value.Substring(1, $end - 1) }
            }
            return ($value -replace '(?:^|\s+)#.*$', '').Trim()
        }
    }
    return ''
}

# The API accepts SMTP_* or Laravel-style MAIL_* settings; either one means a real
# mail server is configured, so Mailpit must not replace it.
$hasCustomSmtp = $false
foreach ($smtpVariable in @('SMTP_HOST', 'MAIL_HOST')) {
    if (-not [string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($smtpVariable)) -or
        -not [string]::IsNullOrWhiteSpace((Get-DotEnvValue -Path $apiEnvFile -Name $smtpVariable))) {
        $hasCustomSmtp = $true
    }
}
$localInboxReady = $false
if (-not $hasCustomSmtp -and (Get-Command docker -CommandType Application -ErrorAction SilentlyContinue)) {
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & docker compose -f (Join-Path $PSScriptRoot 'compose.yaml') up -d --wait mailpit
        $localInboxReady = $LASTEXITCODE -eq 0
    }
    finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($localInboxReady) { Write-Host 'Local email inbox: http://localhost:8025 (invitation emails land here)' }
    else { Write-Warning 'Mailpit did not start; invitation emails will be written to the API log instead.' }
}

$runner = {
    param($directory, $command, $commandArguments, $port, $environment)
    $ErrorActionPreference = 'Stop'
    # Report the job host PID so cleanup also terminates native descendants.
    [pscustomobject]@{ RunnerProcessId = $PID }
    Set-Location -LiteralPath $directory
    # Each job has its own environment; the API's .env does not override PORT.
    $env:PORT = [string]$port
    # Job-only variables; the caller's PowerShell session is never modified.
    if ($environment) { foreach ($name in $environment.Keys) { Set-Item -Path "Env:$name" -Value $environment[$name] } }
    # Windows PowerShell exposes native stderr as error records, including normal logs.
    $ErrorActionPreference = 'Continue'
    & $command @commandArguments 2>&1
    [pscustomobject]@{ ServerExitCode = $LASTEXITCODE }
}

# API hot reload: prefer Air pinned in api/go.mod (`go tool air`), then a global
# `air` on PATH; otherwise plain `go run` (restart run.ps1 after Go changes).
$apiCommand = 'go'
$apiArguments = @('run', './cmd/server')
$apiMode = 'no hot reload'
if (-not $NoReload) {
    $goMod = Get-Content -LiteralPath (Join-Path $apiDirectory 'go.mod') -Raw
    if ($goMod -match '(?m)^\s*tool\s+github\.com/air-verse/air\s*$' -or $goMod -match '(?ms)^tool\s*\(.*?github\.com/air-verse/air.*?\)') {
        $apiCommand = 'go'
        $apiArguments = @('tool', 'air')
        $apiMode = 'hot reload via go tool air'
    }
    elseif (Get-Command air -CommandType Application -ErrorAction SilentlyContinue) {
        $apiCommand = 'air'
        $apiArguments = @()
        $apiMode = 'hot reload via air'
    }
    else {
        Write-Host 'API hot reload is off: Air is not installed. Enable it once with:'
        Write-Host '    cd api; go get -tool github.com/air-verse/air@v1.67.3'
    }
}

$jobs = @()
$runnerProcessIds = @{}
$serverExitCode = 1

# The web app is built with the API port actually chosen above (it may differ
# from -ApiPort when that port is busy). This replaces any NEXT_PUBLIC_API_URL left
# in your session. To point the web app elsewhere, set $env:PAYCORE_API_URL.
$webEnvironment = @{ NEXT_PUBLIC_API_URL = "http://localhost:$ApiPort" }
if (-not [string]::IsNullOrWhiteSpace($env:PAYCORE_API_URL)) { $webEnvironment.NEXT_PUBLIC_API_URL = $env:PAYCORE_API_URL }
# Links in invitation emails point at the web port actually chosen above.
$apiEnvironment = @{}
if ([string]::IsNullOrWhiteSpace($env:APP_URL) -and [string]::IsNullOrWhiteSpace((Get-DotEnvValue -Path $apiEnvFile -Name 'APP_URL'))) { $apiEnvironment.APP_URL = "http://localhost:$WebPort" }
if ($localInboxReady) { $apiEnvironment.SMTP_HOST = '127.0.0.1'; $apiEnvironment.SMTP_PORT = '1025' }
$webStarted = $false
$apiReadyDeadline = [DateTime]::UtcNow.AddSeconds(90)

try {
    $jobs += Start-Job -Name 'PayCore-API' -ScriptBlock $runner -ArgumentList $apiDirectory, $apiCommand, $apiArguments, $ApiPort, $apiEnvironment
    Write-Host 'Starting API; web starts after the API is ready. Press Ctrl+C to stop both.'
    Write-Host "API: http://localhost:$ApiPort ($apiMode)"
    Write-Host "Web: http://localhost:$WebPort"
    Show-WebRoutes -Port $WebPort
    Write-Host 'Registered API endpoints will appear in the API startup logs below.'
    Write-Host 'List routes without starting servers: .\run.ps1 -ListRoutes'

    while ($true) {
        foreach ($job in $jobs) {
            foreach ($item in @(Receive-Job -Job $job -ErrorAction Continue)) {
                if ($null -ne $item.PSObject.Properties['RunnerProcessId']) {
                    $runnerProcessIds[$job.Id] = [int]$item.RunnerProcessId
                }
                elseif ($null -ne $item.PSObject.Properties['ServerExitCode']) {
                    $serverExitCode = [int]$item.ServerExitCode
                }
                else {
                    Write-Host "[$($job.Name)] $item"
                }
            }
        }

        if (@($jobs | Where-Object { $_.State -in @('Completed', 'Failed', 'Stopped') }).Count -gt 0) {
            # Drain final output and the exit code before shutting down the peer.
            if (@($jobs | Where-Object { $_.State -in @('Completed', 'Failed', 'Stopped') -and $_.HasMoreData }).Count -gt 0) {
                continue
            }
            if (@($jobs | Where-Object { $_.State -eq 'Failed' }).Count -gt 0) {
                $serverExitCode = 1
            }
            break
        }
        if (-not $webStarted) {
            $apiIsReady = $false
            try {
                $serviceInfo = Invoke-RestMethod -Uri "http://localhost:$ApiPort/" -TimeoutSec 1 -ErrorAction Stop
                $readiness = Invoke-RestMethod -Uri "http://localhost:$ApiPort/health/ready" -TimeoutSec 1 -ErrorAction Stop
                $apiIsReady = $serviceInfo.service -eq 'PayCore API' -and $readiness.status -eq 'ready'
            }
            catch {
                # Keep draining API logs while Go compiles and PostgreSQL is checked.
            }
            if ($apiIsReady) {
                $jobs += Start-Job -Name 'PayCore-Web' -ScriptBlock $runner -ArgumentList $webDirectory, 'npm.cmd', @('run', 'dev', '--', '--port', [string]$WebPort), $WebPort, $webEnvironment
                $webStarted = $true
                Write-Host "API ready. Starting web with API URL $($webEnvironment.NEXT_PUBLIC_API_URL)."
            }
            if (-not $webStarted -and [DateTime]::UtcNow -ge $apiReadyDeadline) {
                throw 'API did not become ready within 90 seconds; web startup cancelled. Check the API logs above.'
            }
        }
        Start-Sleep -Milliseconds 200
    }
}
finally {
    foreach ($job in $jobs) {
        # A job may have started just before an interruption; collect its PID too.
        foreach ($item in @(Receive-Job -Job $job -ErrorAction SilentlyContinue)) {
            if ($null -ne $item.PSObject.Properties['RunnerProcessId']) {
                $runnerProcessIds[$job.Id] = [int]$item.RunnerProcessId
            }
        }
        if ($runnerProcessIds.ContainsKey($job.Id)) {
            try {
                & taskkill.exe /PID $runnerProcessIds[$job.Id] /T /F 2>$null | Out-Null
            }
            catch {
                # The job host may already have exited.
            }
        }
        Remove-Job -Job $job -Force -ErrorAction SilentlyContinue
    }
}

exit $serverExitCode
