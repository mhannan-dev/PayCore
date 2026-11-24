# Aevum deployment handoff

Status: tooling prepared; no broadcast or live payment is implied by this file.
Signing is performed by the wallet owner on their own machine.

Offline validation:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File contracts/test/deployment-tooling.test.ps1
```

The Solidity deployment checks run with the normal `forge test` suite. The
PowerShell suite uses local RPC fixtures and does not contact or deploy to testnet.

## Prepare

Run from the repository root using PowerShell. Foundry is required. The wrapper
also checks `%USERPROFILE%/.foundry/bin` when Foundry is missing from PATH.
Dependencies must already exist under `contracts/lib`; see
[dependency setup](../docs/quantum/CONTRACTS.md#deployment).

Use a funded testnet wallet. Import it interactively with
`cast wallet import <account-name> --interactive` if a keystore is needed.
Never put private keys, seed phrases or passwords in commands or this repository.

## Simulate, then deploy

Replace the public sender address and account name with your own values:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File contracts/script/deploy-aevum.ps1 -Sender 0x<public-address>
powershell -NoProfile -ExecutionPolicy Bypass -File contracts/script/deploy-aevum.ps1 -Sender 0x<public-address> -Account <keystore-name> -Broadcast
```

Both commands build and test the contracts before simulation. Only the second
command broadcasts. Foundry asks for the keystore password locally. The sender
must match the account. Transactions use legacy type and Shanghai bytecode.
The Solidity script rejects chains other than 44005, missing token bytecode and
tokens whose `decimals()` is not six before broadcasting. `TUSD8_ADDRESS` may
override the default token for a deliberate testnet deployment.

Commit the reviewed contract sources before broadcasting to make the deployed
source reproducible. Preserve Foundry's local `contracts/broadcast` record;
it is git-ignored. A dry run is not a deployment.

## Export actual deployment evidence

After the deployment has at least two confirmations, use its actual transaction hash:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File contracts/script/export-deployment.ps1 -TransactionHash 0x<deployment-tx-hash>
```

The exporter checks the RPC chain, successful creation receipt, transaction,
canonical block, confirmations, deployed bytecode, immutable `token()` and token
decimals. It writes public evidence to `contracts/deployments/aevum-<hash>.json`
and refuses to overwrite earlier evidence. With a token override, also pass
`-TokenAddress 0x<token-address>`. `-RpcUrl` overrides the RPC for both scripts.

The evidence records explorer links, receipt, runtime bytecode, current commit,
working-tree state and source-file SHA-256 hashes. These workspace hashes do not
prove a source-to-bytecode match. Explorer source verification is recorded as
`not_verified`; check availability at deployment time. Two confirmations are a
demo policy, not a finality guarantee.

Return the public contract address and deployment transaction hash after signing.
The subsequent payment demo must record a separate payment transaction and its
`InvoicePaid` log after API/checkout integration. Deployment evidence alone is
not payment proof.
