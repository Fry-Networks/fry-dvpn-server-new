# Fry dVPN — smart contracts

Two Algorand applications (Algorand Python / PuyaPy) form the decentralized
coordination layer. There is no central server: clients discover nodes by reading
`NodeRegistry` boxes directly, and rewards are distributed trustlessly by `RewardPool`.

## Contracts

### NodeRegistry (`smart_contracts/node_registry/contract.py`)
On-chain registry of active VPN miner nodes. One box per node (keyed by the node's
Algorand address); a node may only mutate its own record. Clients enumerate + read the
boxes to discover nodes — no privileged read path. Stale nodes (no heartbeat within
`stale_rounds`, default 640 ≈ 30 min) are permissionlessly reap-able (reaper gets the
box MBR), so the registry self-cleans. See the repo-root `ARCHITECTURE.md` §3.1 for the
`NodeRecord` layout and full ABI.

### RewardPool (`smart_contracts/reward_pool/contract.py`)
Distributes fVPN to nodes proportional to bytes served (Proof-of-Connectivity), per
epoch. Floor-division shares + a `committed` counter guarantee the pool never
over-distributes (`sum(shares) <= epoch_budget <= balance - committed`). Nodes claim
their accrued fVPN; no node can claim more than it accrued.

fVPN = ASA `2485198745` (6 decimals) on mainnet. Tests + LocalNet use a stand-in ASA.

## Build

```bash
cd contracts
uvx puyapy --output-arc56 --out-dir artifacts smart_contracts/node_registry/contract.py
uvx puyapy --output-arc56 --out-dir artifacts smart_contracts/reward_pool/contract.py
```
Artifacts land in each contract's `artifacts/` dir (`*.arc56.json`, `*.approval.teal`).

## Test (LocalNet)

```bash
algokit localnet start
cd contracts
uv run --with algokit-utils --with pytest python -m pytest tests/ -q
```
Covers registration/box-read, heartbeat accumulation, MBR refund, reap staleness,
admin-only gates, proportional distribution, floor-remainder (no over-distribution),
budget-vs-balance guard, and double-claim prevention.

## Deploy

```bash
# LocalNet (safe):
uv run --with algokit-utils python scripts/deploy.py
# Mainnet (deliberate; DEFERRED to operator):
DVPN_NETWORK=mainnet DEPLOYER_MNEMONIC="..." DVPN_CONFIRM_MAINNET=yes \
    uv run --with algokit-utils python scripts/deploy.py
```
Set the printed `NodeRegistry APP_ID` as `REGISTRY_APP_ID` (node daemon) and
`VITE_REGISTRY_APP_ID` (client).

## Escrow settlement (v2, documented, not built)

v1 uses prepaid session payments (client → node fVPN axfer) + PoC-weighted RewardPool
top-ups. A future per-session escrow app would release fVPN to the node as metered bytes
are attested and refund the unused remainder to the client. It would add:
`open_session(client, node, deposit)`, `attest(session, bytes)` (node-signed usage),
`settle(session)` (pay node for consumed bytes, refund remainder). Kept out of v1 to
avoid unnecessary on-chain complexity; the prepaid + rewards model is sufficient and
fully trustless for launch.
