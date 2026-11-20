# Run the project's tests and required static checks from one entry point.
[CmdletBinding()]
param(
    [ValidateSet('all', 'api', 'web')]
    [string]$Suite = 'all'
)

$ErrorActionPreference = 'Stop'

if ($Suite -in @('all', 'api')) {
    Push-Location -LiteralPath (Join-Path $PSScriptRoot 'api')
    try {
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { throw 'API static checks failed.' }
        # Includes central public/integration tests and package-local private tests.
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw 'API tests failed.' }
        if (Get-Command golangci-lint -ErrorAction SilentlyContinue) {
            & golangci-lint run
            if ($LASTEXITCODE -ne 0) { throw 'API lint failed.' }
        }
    }
    finally { Pop-Location }
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
