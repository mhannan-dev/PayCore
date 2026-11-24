[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^0x[0-9a-fA-F]{64}$')]
    [string]$TransactionHash,
    [ValidatePattern('^0x[0-9a-fA-F]{40}$')]
    [string]$TokenAddress = '0x456569838f1d43009a91eba4263147afd7233483',
    [string]$RpcUrl = 'https://rpc-test.qday.info',
    [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Invoke-ChainRpc([string]$Method, [object[]]$Arguments) {
    $body = @{ jsonrpc = '2.0'; id = 1; method = $Method; params = $Arguments } | ConvertTo-Json -Depth 10 -Compress
    $response = Invoke-RestMethod -Uri $RpcUrl -Method Post -ContentType 'application/json' -Body $body -TimeoutSec 30
    if ($response.PSObject.Properties['error']) { throw "RPC failed: $Method" }
    if (-not $response.PSObject.Properties['result']) { throw "Missing RPC result: $Method" }
    return $response.result
}

$chain = Invoke-ChainRpc 'eth_chainId' @()
if ([Convert]::ToUInt64($chain.Substring(2), 16) -ne 44005) { throw 'RPC is not Aevum testnet (44005).' }
$receipt = Invoke-ChainRpc 'eth_getTransactionReceipt' @($TransactionHash)
if ($null -eq $receipt -or $receipt.status -ne '0x1') { throw 'Deployment is not successfully mined.' }
if ($receipt.transactionHash -ne $TransactionHash -or $receipt.contractAddress -notmatch '^0x[0-9a-fA-F]{40}$') {
    throw 'Receipt does not identify this deployment.'
}
$transaction = Invoke-ChainRpc 'eth_getTransactionByHash' @($TransactionHash)
if ($null -eq $transaction -or $transaction.hash -ne $TransactionHash -or $null -ne $transaction.to -or
    $transaction.blockHash -ne $receipt.blockHash -or $transaction.from -ne $receipt.from) {
    throw 'Transaction does not match the contract-creation receipt.'
}
$block = Invoke-ChainRpc 'eth_getBlockByNumber' @($receipt.blockNumber, $false)
if ($null -eq $block -or $block.hash -ne $receipt.blockHash) { throw 'Deployment block is no longer canonical.' }
$head = Invoke-ChainRpc 'eth_blockNumber' @()
$blockNumber = [Convert]::ToUInt64($receipt.blockNumber.Substring(2), 16)
$headNumber = [Convert]::ToUInt64($head.Substring(2), 16)
if ($headNumber -lt $blockNumber -or ($headNumber - $blockNumber + 1) -lt 2) {
    throw 'Wait for at least two confirmations before exporting deployment proof.'
}
$address = $receipt.contractAddress.ToLowerInvariant()
$code = Invoke-ChainRpc 'eth_getCode' @($address, $receipt.blockNumber)
if ($code -notmatch '^0x([0-9a-fA-F]{2})+$') { throw 'No deployed runtime bytecode.' }
$tokenResult = Invoke-ChainRpc 'eth_call' @(@{ to = $address; data = '0xfc0c546a' }, $receipt.blockNumber)
if ($tokenResult -notmatch '^0x0{24}[0-9a-fA-F]{40}$' -or $tokenResult.Substring(26) -ne $TokenAddress.Substring(2)) {
    throw 'Deployed token() does not match the expected token.'
}
$decimals = Invoke-ChainRpc 'eth_call' @(@{ to = $TokenAddress; data = '0x313ce567' }, $receipt.blockNumber)
if ($decimals -notmatch '^0x0{63}6$') { throw 'Token decimals are not six.' }

$root = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$commit = & git -C $root rev-parse HEAD
if ($LASTEXITCODE -ne 0) { throw 'Cannot read source commit.' }
$changes = & git -C $root status --porcelain -- contracts
if ($LASTEXITCODE -ne 0) { throw 'Cannot read contract working-tree status.' }
$sourceHashes = [ordered]@{}
foreach ($file in @('contracts/src/QuantumPayments.sol', 'contracts/script/Deploy.s.sol', 'contracts/foundry.toml')) {
    $sourceHashes[$file] = (Get-FileHash -LiteralPath (Join-Path $root $file) -Algorithm SHA256).Hash.ToLowerInvariant()
}
$manifest = [ordered]@{
    schema_version = 1
    kind = 'quantum_contract_deployment'
    recorded_at = [DateTime]::UtcNow.ToString('o')
    chain_id = 44005
    contract_address = $address
    token_address = $TokenAddress.ToLowerInvariant()
    token_decimals = 6
    transaction_hash = $TransactionHash.ToLowerInvariant()
    deployer_address = $receipt.from.ToLowerInvariant()
    block_number = $blockNumber
    block_hash = $receipt.blockHash
    confirmations = $headNumber - $blockNumber + 1
    explorer_transaction_url = "https://explorer-test.qday.info/tx/$TransactionHash"
    explorer_contract_url = "https://explorer-test.qday.info/address/$address"
    source_commit = $commit.Trim()
    contracts_working_tree_dirty = @($changes).Count -gt 0
    workspace_source_sha256 = $sourceHashes
    source_verification = 'not_verified'
    receipt = $receipt
    runtime_bytecode = $code
}
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $directory = Join-Path $root 'contracts/deployments'
    $null = New-Item -ItemType Directory -Path $directory -Force
    $OutputPath = Join-Path $directory "aevum-$($TransactionHash.ToLowerInvariant()).json"
}
if (Test-Path -LiteralPath $OutputPath) { throw 'Output already exists; choose a new path to preserve the previous proof.' }
$json = $manifest | ConvertTo-Json -Depth 20
# CreateNew prevents a concurrent export from replacing existing evidence.
$stream = [IO.File]::Open($OutputPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write)
try {
    $bytes = [Text.UTF8Encoding]::new($false).GetBytes($json)
    $stream.Write($bytes, 0, $bytes.Length)
} finally {
    $stream.Dispose()
}
Write-Host "Deployment evidence saved: $OutputPath"
