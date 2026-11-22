# Run the project's tests and required static checks from one entry point.
[CmdletBinding()]
param(
    [ValidateSet('all', 'api', 'web')]
    [string]$Suite = 'all',
    [switch]$Integration
)

$ErrorActionPreference = 'Stop'

if ($Suite -in @('all', 'api')) {
    $previousDatabaseTestUrl = $env:DATABASE_TEST_URL
    Push-Location -LiteralPath (Join-Path $PSScriptRoot 'api')
    try {
        if ($Integration -and [string]::IsNullOrWhiteSpace($env:DATABASE_TEST_URL)) {
            # Public development credentials from compose.yaml. Tests use isolated schemas.
            $env:DATABASE_TEST_URL = 'postgres://paycore:paycore_local_dev@127.0.0.1:5434/paycore?sslmode=disable'
        }
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'API static checks failed.' }
        # Includes central public/integration tests and package-local private tests.
        if ($Integration) {
            # Migration tests share an advisory lock; serialize packages to avoid lock starvation.
            & go test -p 1 ./...
        } else {
            & go test ./...
        }
        if ($LASTEXITCODE -ne 0) { throw 'API tests failed.' }
        if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
            & golangci-lint run
            if ($LASTEXITCODE -ne 0) { throw 'API lint failed.' }
        }
    }
    finally {
        $env:DATABASE_TEST_URL = $previousDatabaseTestUrl
        Pop-Location
    }
}

if ($Suite -in @('all', 'web')) {
    Push-Location -LiteralPath (Join-Path $PSScriptRoot 'web')
    try {
        & npm.cmd run check
        if ($LASTEXITCODE -ne 0) { throw 'Frontend static checks failed.' }
        & npm.cmd test
        if ($LASTEXITCODE -ne 0) { throw 'Frontend tests failed.' }
    }
    finally { Pop-Location }
}

Write-Host 'Selected tests and checks passed.'
