# Offline RPC fixtures only. Generated evidence stays in git-ignored cache/.
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$global:PayCoreDeploymentFixture_scenario = 'success'
$global:PayCoreDeploymentFixture_hash = '0x' + ('a' * 64)
$global:PayCoreDeploymentFixture_blockHash = '0x' + ('b' * 64)
$global:PayCoreDeploymentFixture_address = '0x' + ('c' * 40)
$global:PayCoreDeploymentFixture_sender = '0x' + ('d' * 40)
$global:PayCoreDeploymentFixture_token = '456569838f1d43009a91eba4263147afd7233483'

function Invoke-RestMethod {
    param($Uri, $Method, $ContentType, $Body, $TimeoutSec)
    $request = $Body | ConvertFrom-Json
    $result = switch ($request.method) {
        'eth_chainId' { if ($global:PayCoreDeploymentFixture_scenario -eq 'wrong_chain') { '0x1' } else { '0xabe5' } }
        'eth_getTransactionReceipt' {
            [pscustomobject]@{
                transactionHash = $global:PayCoreDeploymentFixture_hash
                status = $(if ($global:PayCoreDeploymentFixture_scenario -eq 'failed') { '0x0' } else { '0x1' })
                contractAddress = $global:PayCoreDeploymentFixture_address
                from = $global:PayCoreDeploymentFixture_sender
                blockNumber = '0x10'
                blockHash = $global:PayCoreDeploymentFixture_blockHash
            }
        }
        'eth_getTransactionByHash' {
            [pscustomobject]@{ hash = $global:PayCoreDeploymentFixture_hash; to = $null; blockHash = $global:PayCoreDeploymentFixture_blockHash; from = $global:PayCoreDeploymentFixture_sender }
        }
        'eth_getBlockByNumber' {
            [pscustomobject]@{ hash = $(if ($global:PayCoreDeploymentFixture_scenario -eq 'reorg') { '0xdead' } else { $global:PayCoreDeploymentFixture_blockHash }) }
        }
        'eth_blockNumber' { if ($global:PayCoreDeploymentFixture_scenario -eq 'unconfirmed') { '0x10' } else { '0x12' } }
        'eth_getCode' { if ($global:PayCoreDeploymentFixture_scenario -eq 'missing_code') { '0x' } else { '0x6000' } }
        'eth_call' {
            if ($request.params[0].data -eq '0xfc0c546a') {
                if ($global:PayCoreDeploymentFixture_scenario -eq 'wrong_token') { '0x' + ('0' * 64) } else { '0x' + ('0' * 24) + $global:PayCoreDeploymentFixture_token }
            } else {
                '0x' + ('0' * 63) + $(if ($global:PayCoreDeploymentFixture_scenario -eq 'wrong_decimals') { '8' } else { '6' })
            }
        }
        default { throw "Unexpected RPC method: $($request.method)" }
    }
    [pscustomobject]@{ jsonrpc = '2.0'; id = 1; result = $result }
}

$exporter = Join-Path $PSScriptRoot '../script/export-deployment.ps1'
$directory = Join-Path $PSScriptRoot '../cache'
$null = New-Item -ItemType Directory -Path $directory -Force
$output = Join-Path $directory "deployment-fixture-$([Guid]::NewGuid().ToString('N')).json"
try {
    $expectedErrors = [ordered]@{
        wrong_chain = 'RPC is not Aevum testnet'
        failed = 'Deployment is not successfully mined'
        reorg = 'Deployment block is no longer canonical'
        unconfirmed = 'Wait for at least two confirmations'
        missing_code = 'No deployed runtime bytecode'
        wrong_token = 'Deployed token\(\) does not match'
        wrong_decimals = 'Token decimals are not six'
    }
    foreach ($case in $expectedErrors.Keys) {
        $global:PayCoreDeploymentFixture_scenario = $case
        $rejected = $false
        try { & $exporter -TransactionHash $global:PayCoreDeploymentFixture_hash -OutputPath $output } catch {
            if ($_.Exception.Message -notmatch $expectedErrors[$case]) { throw }
            $rejected = $true
        }
        if (-not $rejected -or (Test-Path -LiteralPath $output)) { throw "Invalid evidence accepted: $case" }
        Write-Host "PASS rejects $case"
    }
    $global:PayCoreDeploymentFixture_scenario = 'success'
    & $exporter -TransactionHash $global:PayCoreDeploymentFixture_hash -OutputPath $output
    $manifest = Get-Content -LiteralPath $output -Raw | ConvertFrom-Json
    if ($manifest.chain_id -ne 44005 -or $manifest.confirmations -ne 3 -or
        $manifest.contract_address -ne $global:PayCoreDeploymentFixture_address -or $manifest.source_verification -ne 'not_verified') {
        throw 'Valid deployment evidence has incorrect metadata.'
    }
    Write-Host 'PASS exports verified receipt metadata'
    $before = (Get-FileHash -LiteralPath $output).Hash
    $rejected = $false
    try { & $exporter -TransactionHash $global:PayCoreDeploymentFixture_hash -OutputPath $output } catch { $rejected = $true }
    if (-not $rejected -or (Get-FileHash -LiteralPath $output).Hash -ne $before) { throw 'Existing proof was overwritten.' }
    Write-Host 'PASS preserves existing evidence'
} finally {
    if (Test-Path -LiteralPath $output) { Remove-Item -LiteralPath $output }
}

