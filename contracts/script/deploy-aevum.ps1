[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^0x[0-9a-fA-F]{40}$')]
    [string]$Sender,
    [string]$Account,
    [string]$RpcUrl = 'https://rpc-test.qday.info',
    [switch]$Broadcast
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Locate Foundry on Windows even when a newly installed binary is not yet on PATH.
$forgeCommand = Get-Command forge -ErrorAction SilentlyContinue
$forge = if ($forgeCommand) { $forgeCommand.Source } else { Join-Path $env:USERPROFILE '.foundry/bin/forge.exe' }
if (-not (Test-Path -LiteralPath $forge)) { throw 'Install Foundry before running deployment.' }
if ($Broadcast -and [string]::IsNullOrWhiteSpace($Account)) {
    throw 'Broadcast requires -Account with a Foundry keystore name. Signing stays in the local keystore.'
}

Push-Location (Join-Path $PSScriptRoot '..')
try {
    & $forge build
    if ($LASTEXITCODE -ne 0) { throw 'Contract build failed.' }
    & $forge test
    if ($LASTEXITCODE -ne 0) { throw 'Contract tests failed.' }

    $arguments = @('script', 'script/Deploy.s.sol:Deploy', '--legacy', '--rpc-url', $RpcUrl,
        '--chain', '44005', '--sender', $Sender, '--evm-version', 'shanghai')
    if ($Broadcast) { $arguments += @('--account', $Account, '--broadcast') }
    & $forge @arguments
    if ($LASTEXITCODE -ne 0) { throw 'Deployment simulation or broadcast failed.' }

    if ($Broadcast) {
        Write-Host 'Broadcast completed. Export proof using the mined deployment transaction hash:'
        Write-Host '.\script\export-deployment.ps1 -TransactionHash 0x<actual-hash>'
    } else {
        Write-Host 'Dry-run completed; no transaction was broadcast. Add -Broadcast -Account <name> to deploy.'
    }
} finally {
    Pop-Location
}
