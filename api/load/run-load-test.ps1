[CmdletBinding()]
param(
    [string]$BaseUrl = 'http://127.0.0.1:8080',
    [string]$Email = 'operator@yopmail.com',
    [string]$AuditorEmail = 'auditor@yopmail.com',
    [ValidateRange(1, 1000)]
    [int]$Vus = 20,
    [string]$Duration = '30s',
    [string]$Currency = 'JPY',
    [string]$DebitAccount = '1001',
    [string]$CreditAccount = '2003',
    [ValidateRange(1, 1000000)]
    [int]$Amount = 1,
    [ValidateRange(0, 60)]
    [double]$SleepSeconds = 0,
    [ValidateRange(0, 1000000)]
    [int]$MaxBusy = 0,
    [ValidateRange(1, 600000)]
    [int]$MaxP95Ms = 2000,
    [ValidateRange(0.0001, 1)]
    [double]$MaxFailureRate = 0.01
)

$ErrorActionPreference = 'Stop'

$useDocker = -not (Get-Command k6 -ErrorAction SilentlyContinue)
if ($useDocker -and -not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'k6 is required. Install k6 or Docker, then run this script again.'
}
$k6Image = if ([string]::IsNullOrWhiteSpace($env:K6_IMAGE)) { 'grafana/k6:latest' } else { $env:K6_IMAGE }

$previousPassword = $env:PAYCORE_LOAD_PASSWORD
$password = $previousPassword
if ([string]::IsNullOrWhiteSpace($password)) {
    $secure = Read-Host 'PayCore load-test password' -AsSecureString
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $password = [Runtime.InteropServices.Marshal]::PtrToStringAuto($bstr)
    }
    finally {
        [Runtime.InteropServices.Marshal]::FreeBSTR($bstr)
    }
}
if ([string]::IsNullOrWhiteSpace($password)) {
    throw 'A load-test password is required.'
}

$env:BASE_URL = $BaseUrl
$env:EMAIL = $Email
$env:AUDITOR_EMAIL = $AuditorEmail
$env:CURRENCY = $Currency
$env:DEBIT_ACCOUNT = $DebitAccount
$env:CREDIT_ACCOUNT = $CreditAccount
$env:VUS = [string]$Vus
$env:DURATION = $Duration
$env:AMOUNT = [string]$Amount
$env:SLEEP_SECONDS = [string]$SleepSeconds
$env:MAX_BUSY = [string]$MaxBusy
$env:MAX_P95_MS = [string]$MaxP95Ms
$env:MAX_FAILURE_RATE = [string]$MaxFailureRate
$env:RUN_ID = [guid]::NewGuid().ToString('N')
$env:PAYCORE_LOAD_PASSWORD = $password

Push-Location -LiteralPath $PSScriptRoot
try {
    if ($useDocker) {
        & docker run --rm -v "${PSScriptRoot}:/scripts" -w /scripts `
            -e BASE_URL -e EMAIL -e AUDITOR_EMAIL -e PAYCORE_LOAD_PASSWORD -e CURRENCY `
            -e DEBIT_ACCOUNT -e CREDIT_ACCOUNT -e VUS -e DURATION -e AMOUNT `
            -e SLEEP_SECONDS -e MAX_BUSY -e MAX_P95_MS -e MAX_FAILURE_RATE -e RUN_ID `
            $k6Image run /scripts/k6-transfer-load.js
    }
    else {
        & k6 run .\k6-transfer-load.js
    }
    if ($LASTEXITCODE -ne 0) { throw 'k6 load test failed.' }
}
finally {
    Pop-Location
    $env:PAYCORE_LOAD_PASSWORD = $previousPassword
}
