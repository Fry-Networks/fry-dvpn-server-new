# frynode - Fry dVPN Miner Node Daemon

A decentralized VPN miner node daemon that manages WireGuard connections, validates consumer payments, and reports bandwidth metrics to an Algorand smart contract.

## Features

- **Algorand Integration**: Registers nodes on-chain, validates fVPN token payments, submits heartbeats
- **WireGuard Management**: Creates and manages peer connections over WireGuard
- **HTTP API**: Provides challenge/response authentication and session management
- **IP Pool Management**: Allocates IPs from a configurable subnet (default 10.7.0.0/24)
- **Egress Policy**: Configurable firewall rules to prevent abuse
- **Bandwidth Metering**: Tracks per-peer bandwidth and submits metrics on-chain

## Building

### Prerequisites

- Go 1.26+
- Access to Algorand node (LocalNet, testnet, or mainnet)

### Build Command

```bash
export PATH="$PATH:/c/Program Files/Go/bin"
export GOFLAGS=-mod=mod
cd node
go mod tidy
go build ./...
```

## Configuration

All configuration is environment-driven (no hardcoded values):

| Variable | Default | Description |
|----------|---------|-------------|
| `ALGOD_SERVER` | `http://localhost` | Algorand node address |
| `ALGOD_PORT` | `4001` | Algorand node port |
| `ALGOD_TOKEN` | 64 a's | Algorand node token |
| `REGISTRY_APP_ID` | (required) | NodeRegistry smart contract app ID |
| `FVPN_ASA_ID` | `2485198745` | fVPN token asset ID |
| `NODE_MNEMONIC` | (generated) | Algorand account mnemonic |
| `WG_PRIVATE_KEY` | (generated) | WireGuard private key |
| `PUBLIC_ENDPOINT` | (derives from IP + WG_PORT) | Advertised endpoint |
| `REGION` | (required) | Node region |
| `CAPACITY_MBPS` | (required) | Node capacity in Mbps |
| `PRICE_PER_GB` | (required) | Price in fVPN microunits per GB |
| `WG_PORT` | `51820` | WireGuard listening port |
| `API_PORT` | `8088` | HTTP API port |
| `HEARTBEAT_SECONDS` | `300` | Heartbeat interval |
| `ALLOW_FULL_TUNNEL` | `false` | Allow full-tunnel egress (see security note) |

### Example: LocalNet

```bash
export ALGOD_SERVER=http://localhost
export ALGOD_PORT=4001
export ALGOD_TOKEN="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
export REGISTRY_APP_ID=1002
export REGION="us-west"
export CAPACITY_MBPS=100
export PRICE_PER_GB=1000000
export PUBLIC_ENDPOINT="localhost:51820"

go run ./main.go
```

## API Endpoints

### `GET /health`
Returns node health status.

```json
{
  "status": "healthy",
  "wg_iface": "wg0",
  "peers": 3,
  "registered": true,
  "last_heartbeat_round": 12345,
  "version": "0.1.0"
}
```

### `GET /challenge?wallet=<algo_address>`
Initiates authentication challenge.

Request:
```
GET /challenge?wallet=AQXXXXXXXXXX...
```

Response:
```json
{
  "challenge": "base64_encoded_32byte_nonce",
  "expires_unix": 1234567890
}
```

### `POST /session`
Creates a new session after verifying signature and payment.

Request:
```json
{
  "wallet_address": "AQXXXXXXXXXX...",
  "challenge": "base64_challenge",
  "signature": "base64_ed25519_signature_over_nonce",
  "payment_txid": "txn_id_of_fVPN_transfer",
  "client_wg_pubkey": "base64_wireguard_public_key"
}
```

Response:
```json
{
  "session_id": "base64_session_id",
  "interface_address": "10.7.0.1/32",
  "node_wg_pubkey": "base64_node_wg_public_key",
  "node_endpoint": "node.example.com:51820",
  "allowed_ips": ["0.0.0.0/1", "128.0.0.0/1"],
  "dns": "1.1.1.1",
  "persistent_keepalive": 25
}
```

### `DELETE /session/<session_id>`
Closes a session and removes the peer.

Response: `204 No Content`

## WireGuard Implementation

Currently uses a mock WireGuard controller for testing. Production deployment requires:

- Linux kernel with WireGuard module, OR
- `wireguard-go` userspace implementation

The daemon uses the `golang.zx2c4.com/wireguard` package (wgctrl) which abstracts over both kernel and userspace implementations.

## Security Notes

### Egress Policy (Abuse Mitigation)

The node implements egress filtering to prevent abuse:

- **DenyRFC1918** (default): Blocks RFC1918 private ranges (10/8, 172.16/12, 192.168/16), common abuse ports (23, 25, 53, 67, 68)
- **DenyAbuse**: More aggressive filtering
- **AllowAll**: Full tunnel mode (operator responsibility)

Set `ALLOW_FULL_TUNNEL=true` to enable full-tunnel mode. In this case:

- Generated rules are written to `node-identity/egress.rules`
- Operator must configure firewall manually (iptables, nftables, or vendor-specific tools)
- Operator assumes full responsibility for abuse prevention

### Key Management

- Algorand mnemonic and WireGuard private key stored in `node-identity/` with mode `0600`
- Keys are NEVER logged or printed
- Regenerate keys via environment variable override (they persist on disk)

### Per-Wallet Limits

- Maximum 3 concurrent sessions per wallet
- Token bucket rate limiting on `/session` and `/challenge` (configurable)
- Payment verification required for every session

### Data Persistence

Identity files (keys) are persisted in `node-identity/`:

```
node-identity/
  ├── account.txt       (mnemonic, 0600)
  ├── wg.key           (WG private key, 0600)
  └── egress.rules     (generated firewall rules)
```

## Testing

All modules include comprehensive unit tests.

```bash
go test ./...
```

Test coverage includes:

- **ippool**: allocation, free, exhaustion, boundary conditions
- **identity**: generation, persistence, reload, public key derivation
- **wg**: mock controller, peer lifecycle, stats tracking
- **egress**: policy generation, rule validation
- **registry**: transaction building, encoding
- **metering**: session tracking, bandwidth metering
- **api**: challenge generation, session lifecycle, handler routing

## Development

### Adding a New Module

1. Create `module_name/module_name.go`
2. Define public API (interfaces preferred)
3. Add unit tests in `module_name/module_name_test.go`
4. Update main.go to wire the module

### Extending the API

HTTP handlers are registered in `api.go` via `RegisterHandlers()`. Add new endpoints by:

1. Defining a handler method on `Server`
2. Registering in `RegisterHandlers()`
3. Adding tests in `api_test.go`

## Implemented

- Real on-chain registration/heartbeat/deregistration via the Algorand SDK Atomic
  Transaction Composer against the NodeRegistry app (verified against LocalNet by
  `contracts/scripts/node_e2e.py`).
- Real Algorand account + Curve25519 WireGuard keypair generation/persistence.
- fVPN payment verification before provisioning a peer.
- Ed25519 wallet-signature challenge auth, per-wallet session cap, and per-IP
  token-bucket rate limiting on `/challenge` and `/session`.

## Operational notes

- The WireGuard data plane uses the `wg.Controller` interface. A real deployment
  wires the `wgctrl`-backed controller (kernel WireGuard on Linux, or `wireguard-go`
  userspace on Windows/macOS); the in-memory controller is used for tests and for
  hosts without a WireGuard interface.
- No TLS on the HTTP API — run the node behind the operator's reverse proxy / the
  Tailscale network, or terminate TLS upstream.
- Sessions are in-memory (re-established by clients after a node restart).
- On non-POSIX hosts, graceful deregistration on shutdown is best-effort; stale
  records are reclaimed on-chain by `reap_stale`.

## License

Part of the Fry Networks dVPN system.
