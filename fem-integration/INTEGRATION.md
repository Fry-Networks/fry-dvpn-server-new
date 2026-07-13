# FryVPN Integration for Fry Edge Miner

## Overview

This is a drop-in Rust integration module that adds Fry dVPN (frynode) support to the Fry Edge Miner (FEM) desktop application. It follows the same pattern as existing integrations like `mysterium.rs`, providing process supervision, health checking, and UI metadata.

## Integration Steps

### 1. Copy the integration module into FEM

Copy `src/fryvpn.rs` from this crate into the FEM repository:

```bash
cp fem-integration/src/fryvpn.rs fry-edge-miner/src-tauri/src/integrations/fryvpn.rs
```

### 2. Register the integration in the integrations module

In `fry-edge-miner/src-tauri/src/integrations/mod.rs`, add:

```rust
mod fryvpn;
pub use fryvpn::FryVpnIntegration;
```

Then add to the integration registry (typically in the supervisor or manager module):

```rust
let integrations = vec![
    Box::new(FryVpnIntegration::new()),
    // ... other integrations
];
```

### 3. Add UI metadata to integrationMeta.ts

In `fry-edge-miner/src/lib/integrationMeta.ts`, add the FryVPN entry:

```typescript
export const integrationMeta: Record<string, IntegrationMetadata> = {
  fryvpn: {
    id: "fryvpn",
    name: "Fry dVPN",
    description: "Fry decentralized VPN node that provides bandwidth to the Fry network",
    category: "Bandwidth",
    icon: "fryvpn-icon", // Use appropriate icon name
  },
  // ... other integrations
};
```

### 4. Add UI card component to Integrations.tsx

In `fry-edge-miner/src/pages/Integrations.tsx`, add the FryVPN card:

```typescript
{meta.category === "Bandwidth" && (
  <IntegrationCard
    integration={meta}
    status={statusMap["fryvpn"]}
    onToggle={() => toggleIntegration("fryvpn")}
  />
)}
```

Or use the existing integration loop if one exists:

```typescript
Object.entries(integrationMeta).map(([id, meta]) => (
  <IntegrationCard
    key={id}
    integration={meta}
    status={statusMap[id]}
    onToggle={() => toggleIntegration(id)}
  />
))
```

## Architecture

### Supervised Trait

The `FryVpnIntegration` implements the `Supervised` trait which defines:

- `spawn_command()` — builds a `std::process::Command` to start the frynode binary
- `id()` — returns "fryvpn"
- `display_name()` — returns "Fry dVPN"
- `health_url()` — returns "http://127.0.0.1:8088/health"

### Process Supervision

The `ProcessSupervisor` manages the frynode child process:

- `start()` — spawns the process
- `stop()` — kills the process gracefully
- `is_running()` — checks if the process is still alive

### Health Checking

The `HealthChecker` periodically polls the health endpoint:

```
GET http://127.0.0.1:8088/health
{
  "status": "healthy",
  "registered": true,
  "last_heartbeat_round": 12345,
  ...
}
```

Returns `ProcessStatus::Running` only if `status == "healthy"` AND `registered == true`.

### Environment Variables

The integration passes the following environment variables to frynode if set:

- `REGISTRY_APP_ID` — Algorand application ID for the registry
- `ALGOD_SERVER` — Algorand node endpoint
- `REGION` — Geographic region identifier
- `PRICE_PER_GB` — Price per gigabyte of bandwidth
- `WG_PORT` — WireGuard port
- `API_PORT` — API server port

Binary location can be overridden via `FRYNODE_BIN` env var; defaults to `frynode.exe` on Windows, `frynode` on Unix.

## Testing

All tests are offline and deterministic:

```bash
cd fem-integration
cargo test
```

Tests include:
- Health status parsing (Running, Unhealthy, invalid JSON)
- Integration metadata serialization
- Environment variable passthrough
- Process lifecycle (start, stop, is_running)

## Pattern Matching with mysterium.rs

The implementation mirrors the Mysterium integration pattern:

| Aspect | mysterium.rs | fryvpn.rs |
|--------|--------------|-----------|
| Health endpoint | HTTP GET to status URL | HTTP GET to 127.0.0.1:8088/health |
| Status parsing | Parse JSON response | Parse status + registered fields |
| Process supervision | ProcessSupervisor<Self> | ProcessSupervisor<Self> |
| Env vars | Passed to child | Passed to child |
| UI metadata | hardcoded strings | meta.rs module |

## Notes

- The health checker uses `ureq` (blocking HTTP) to avoid async complexity in tests
- Mock `HealthFetcher` trait allows offline unit testing
- All error types use `thiserror` for ergonomic error handling
- Zero unsafe code, no network calls in normal tests
