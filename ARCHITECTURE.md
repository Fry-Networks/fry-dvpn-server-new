# Fry dVPN — Decentralized Architecture

> Status: rebuild of the Octaloop delivery into a properly decentralized VPN.
> This document is the source of truth for the node/miner side, the on-chain
> coordination layer, and how the consumer client interacts with them.

## 1. Problem with the delivered system

The Octaloop delivery was a **centralized** VPN wearing a "dVPN" label:

- One hardcoded WireGuard server (`54.211.138.164:8000`) that all clients hit over plaintext HTTP.
- A single `ip_pool.json` peer factory (`main.py` + `wgrest`), full-tunnel `0.0.0.0/0` for every peer — which is exactly how it got weaponized into a 262k-request SYN-flood relay.
- No on-chain anything: no node registry, no payments, no Proof-of-Connectivity, no rewards.
- Committed secrets (WG private keys, 131 peer configs, a live Mongo URI, a wallet mnemonic).

## 2. Target model

Three parts, **no central server of any kind**:

```
   Consumer client                 Algorand (coordination)              Miner node (this repo)
   ───────────────                 ──────────────────────               ──────────────────────
   discover nodes  ───reads────▶   NodeRegistry app                ◀──  register_node / update_node
   (algod/indexer box reads)        one box per active node         ◀──  heartbeat(bytes, round)  [PoC]
   rank by region+latency+load                                           WireGuard endpoint (wgctrl)
   pay fVPN session ──axfer───▶     (payment proof on-chain)  ──────▶    verify sig + payment
   WireGuard tunnel ════════════════════════════════════════════════▶   provision peer, route, meter
   failover on stale/timeout        RewardPool (fVPN, PoC-weighted) ─▶   node earns ∝ bytes served
```

- **No central relay:** each miner node is itself a WireGuard endpoint. Consumer traffic goes client → node → internet. Traffic never transits a Fry-owned hub.
- **No central discovery API:** the list of available nodes lives in Algorand box storage. Clients read it directly from any public algod/indexer. There is no server whose downtime removes discovery.
- **No central auth:** a node authenticates a consumer by verifying (a) an Ed25519 signature over a challenge proving control of the paying Algorand address, and (b) an on-chain fVPN payment. No username/password, no auth server.
- **No single point of failure:** if a node dies, its registry entry goes stale (heartbeat age > `STALE_ROUNDS`) and clients skip/fail-over to another node. `reap_stale` reclaims the box.

## 3. On-chain coordination layer

Token: **fVPN — ASA `2485198745`** (unit `fVPN`, 6 decimals). All payments and rewards are denominated in fVPN µunits (1 fVPN = 1_000_000 µ).

### 3.1 NodeRegistry (Algorand application, PuyaPy)

Box storage, one box per node keyed by the node's 32-byte Algorand public key.

Box value (ARC-4 struct `NodeRecord`):

| field | type | meaning |
|---|---|---|
| `wg_pubkey` | `arc4.StaticArray[Byte, 32]` | node's WireGuard public key (Curve25519) |
| `endpoint` | `arc4.String` | `host:port` the client dials (public IP/DNS + UDP port) |
| `region` | `arc4.String` | ISO-ish region code, e.g. `us-east`, `eu-west` |
| `capacity_mbps` | `arc4.UInt32` | advertised bandwidth capacity |
| `price_per_gb` | `arc4.UInt64` | fVPN µunits per GB served |
| `last_heartbeat` | `arc4.UInt64` | Algorand round of the last heartbeat |
| `cumulative_bytes` | `arc4.UInt64` | lifetime bytes served (PoC accounting) |
| `active_sessions` | `arc4.UInt32` | current live sessions (load signal) |
| `status` | `arc4.UInt8` | 0=inactive, 1=active, 2=draining |

Methods (ABI):

- `register_node(wg_pubkey, endpoint, region, capacity_mbps, price_per_gb)` — creates the caller's box. Requires the caller pays the box MBR via a grouped payment txn. Only `Txn.sender`'s own box.
- `update_node(endpoint, region, capacity_mbps, price_per_gb)` — mutate own box.
- `heartbeat(bytes_served_delta, active_sessions)` — bumps `last_heartbeat = Global.round`, adds to `cumulative_bytes`, sets load. This is the Proof-of-Connectivity signal.
- `deregister_node()` — deletes own box, refunds MBR to caller.
- `reap_stale(node_pubkey)` — anyone may call; if `Global.round - last_heartbeat > STALE_ROUNDS`, deletes the box (MBR goes to the reaper as an incentive) . Keeps the registry self-cleaning.
- Read path: **none needed on-chain** — clients enumerate boxes via `algod GET /v2/applications/{id}/boxes` + `.../box/{name}` or via indexer. Discovery is trustless and server-independent.

Auth invariants: every mutating method checks the box name == `Txn.sender` public key (a node can only touch its own record). `reap_stale` is permissionless but gated on staleness. Admin (creator) may `pause()` / `set_stale_rounds()` for emergency only; admin cannot edit node records or seize funds.

### 3.2 Payments & rewards

- **Session payment (v1, prepaid):** client sends an fVPN `axfer` directly to the node's address for `ceil(expected_gb) * price_per_gb`. The node validates the txn (asset id == fVPN, receiver == node, amount ≥ price) before provisioning the peer. On-chain receipt = the txid; the node stores it against the session.
- **Rewards (PoC-weighted):** a **RewardPool** app (patterned on the existing FryMinerRewardPool V2, N-token capable) distributes fVPN to nodes proportional to `cumulative_bytes` growth attested by heartbeats over an epoch. Reuses the proven reward-pool math (no V1 truncation bug). Distribution is a deferred/manual admin action; the contract + script ship here.
- **Escrow settlement (v2, documented not built):** a per-session escrow app that releases fVPN to the node as metered bytes are attested, refunding the unused remainder to the client. Design captured in `contracts/README.md`.

Amount safety (per CLAUDE.md §15): all fee txns are built after `assignGroupID`; withdrawal ≤ deposited is asserted; tiny/large/non-divisible amounts are unit-tested; privileged methods assert `Txn.sender == admin`.

## 4. Miner node daemon (`node/`, Go)

A single static binary supervised by FEM (see §5). Responsibilities:

1. **Identity:** on first run, generate a WireGuard keypair and an Algorand account (or load from an OS-protected path, `0600`, never committed). Print only the public key + address.
2. **Registration:** on start, call `register_node` with its public endpoint, region, capacity, price. Every `HEARTBEAT_INTERVAL` (default 5 min) call `heartbeat` with the byte delta measured from `wgctrl` and current session count (Proof-of-Connectivity).
3. **Provisioning API** (localhost or LAN, authenticated): `POST /session` with `{wallet_address, signature, challenge, payment_txid}`. The node:
   - verifies the Ed25519 signature over the challenge for `wallet_address`,
   - verifies `payment_txid` is a confirmed fVPN payment to the node ≥ price,
   - allocates an IP from its local pool, adds a WireGuard peer via `wgctrl`, returns the client config (address, node pubkey, endpoint, allowed IPs, DNS).
4. **Metering:** poll `wgctrl` peer transfer counters; feed real bytes into heartbeats and enforce session quotas; expire idle/over-quota peers.
5. **Abuse mitigation:** peers are **not** blanket `0.0.0.0/0`. Default egress policy blocks RFC1918 + known-abuse ports; full-tunnel is an opt-in with documented egress firewalling. Per-wallet peer caps + rate limits retained. Keeps the incident from recurring.

Packages: `golang.zx2c4.com/wireguard/wgctrl` (peer mgmt + stats), `github.com/algorand/go-algorand-sdk/v2` (registration/heartbeat/tx verification), stdlib `net/http` (provisioning API), `golang.org/x/crypto` (Ed25519 challenge). Userspace `wireguard-go` fallback where no kernel module (Windows/macOS).

## 5. FEM integration (`fem-integration/`, Rust)

The node runs on FEM devices as a partner integration, exactly like Mysterium/Presearch in `fry-edge-miner`:

- `fryvpn.rs` implements the same integration contract as `src-tauri/src/integrations/mysterium.rs`: `start()`, `stop()`, `status()`, `health()`, toggleable, supervised by the FEM `supervisor` (`process.rs`/`health.rs`/`platform.rs`). It launches the Go `frynode` binary as a supervised child, health-checks it via the daemon `/health` endpoint **and** on-chain heartbeat freshness.
- `integration_meta` entry (id `fryvpn`, name "Fry dVPN", category "Bandwidth", icon) for the FEM UI card.
- `INTEGRATION.md` gives the exact diff to register it inside `fry-edge-miner` (`integrations/mod.rs`, `Integrations.tsx`). Shipped as a drop-in; the live FEM repo is not modified in this run.

## 6. Consumer client (separate repo `fry-dvpn-client-new`, Electron/React)

- Discovery: reads NodeRegistry boxes via algod/indexer → node list.
- Selection + failover: rank by region preference, measured WireGuard handshake latency, and load (`active_sessions`); auto-reconnect to next-best on stale heartbeat or handshake timeout.
- Payment: fVPN session `axfer` to the selected node; wallet-signature challenge for auth.
- Real metrics from the WireGuard interface counters (replaces the simulated data).
- No central Mongo dependency for the core flow; local encrypted store for wallet/session.

## 7. Security posture

- No secrets in the repo (keys/pools/logs `.gitignore`d and generated at runtime).
- WireGuard keys generated per-node/per-client, never transmitted in plaintext beyond the tunnel setup, never committed.
- Node authenticates consumers cryptographically + by on-chain payment; no open unauthenticated peer creation.
- Egress policy on nodes prevents the relay-abuse class of attack.
- Admin powers on-chain are limited to pause/param — cannot seize node funds or edit records.

## 8. What is deferred (needs operator action / outside autonomous permission)

- Mainnet deployment + funding of NodeRegistry and RewardPool (scripts provided; QA uses LocalNet).
- Git-history secret scrub + rotation of the exposed Mongo password / any exposed wallet.
- Insertion of the FEM module into the live `fry-edge-miner` repo.
- A real multi-node mainnet fleet and live mainnet tunnel E2E.
