# FryVPN Integration Module

A self-contained Rust crate that provides Fry dVPN (frynode) integration for the Fry Edge Miner (FEM) desktop application.

## What is this?

This crate implements a supervisor module that allows FEM to manage the frynode dVPN binary as a child process, similar to how FEM supervises Mysterium, Presearch, and other bandwidth provider integrations.

**Key features:**

- Full process lifecycle management (start, stop, health check)
- Health endpoint polling (`http://127.0.0.1:8088/health`)
- Environment variable passthrough (REGISTRY_APP_ID, ALGOD_SERVER, REGION, etc.)
- Serializable metadata for UI rendering
- Comprehensive unit tests with mocked HTTP fetcher (100% offline)
- Zero unsafe code, minimal dependencies

## Building

Requires `cargo` 1.96.0+:

```bash
cargo build
```

This produces `target/debug/libfryvpn_integration.rlib` (library) and the public API in `src/lib.rs`.

## Testing

All tests are deterministic and offline (no network calls):

```bash
cargo test
```

**Test coverage (13 tests):**
- Health status parsing (Running, Unhealthy, invalid JSON, missing fields)
- Integration metadata (creation, serialization, deserialization)
- Process supervision (spawn, environment variables)
- Display names and IDs

## Code Quality

```bash
cargo clippy -- -D warnings
```

Passes without warnings. Code is idiomatic Rust with proper error handling via `thiserror`.

## Module Structure

```
src/
├── lib.rs           # Public API exports
├── supervisor.rs    # Trait definitions & core logic
│   ├── Supervised trait
│   ├── ProcessSupervisor<S>
│   ├── HealthChecker<S, F>
│   ├── HealthFetcher trait (for testing)
│   └── SupervisorError
├── fryvpn.rs        # FryVPN implementation
│   └── FryVpnIntegration
├── meta.rs          # UI metadata
│   └── IntegrationMeta
└── tests/           # Unit tests for all modules
```

## Usage as a Library

To use this in the FEM crate:

```rust
use fryvpn_integration::{FryVpnIntegration, ProcessSupervisor};

let mut supervisor = FryVpnIntegration::new().start()?;
let status = supervisor.integration.check_health()?;
```

Or add to `Cargo.toml`:

```toml
[dependencies]
fryvpn-integration = { path = "../fem-integration" }
```

## Integration with FEM

See `INTEGRATION.md` for step-by-step instructions to add this into the live Fry Edge Miner repository.

**Summary:**
1. Copy `src/fryvpn.rs` → `fry-edge-miner/src-tauri/src/integrations/fryvpn.rs`
2. Register in `integrations/mod.rs`
3. Add metadata to `integrationMeta.ts`
4. Add UI card to `Integrations.tsx`

## Health Endpoint

The module expects frynode to serve a health endpoint at `http://127.0.0.1:8088/health`:

```json
{
  "status": "healthy",
  "registered": true,
  "last_heartbeat_round": 12345
}
```

The integration reports:
- **Running** — if `status == "healthy"` AND `registered == true`
- **Unhealthy** — otherwise (includes unregistered or down status)

## Environment Variables

Passed to the frynode binary if set:

- `REGISTRY_APP_ID` — Algorand registry contract ID
- `ALGOD_SERVER` — Algorand node URL
- `REGION` — Geographic region
- `PRICE_PER_GB` — Bandwidth price
- `WG_PORT` — WireGuard listening port
- `API_PORT` — API server port

Binary path: set `FRYNODE_BIN` or defaults to `frynode.exe` (Windows) / `frynode` (Unix).

## Architecture Notes

### Trait-Based Design

The supervisor pattern uses a `Supervised` trait so different integrations (Mysterium, Presearch, FryVPN) can be swapped without changing the supervisor logic.

### Mock-Friendly Testing

The `HealthFetcher` trait allows tests to inject a mock HTTP client, so tests never hit the network. Real implementation uses `ureq` (blocking HTTP).

### Error Handling

All fallible operations return `Result<T, SupervisorError>` using `thiserror` for ergonomic error chaining.

## Files

| File | Purpose |
|------|---------|
| `Cargo.toml` | Dependency manifest |
| `src/lib.rs` | Module exports |
| `src/supervisor.rs` | Core trait + generic supervisor + health checker |
| `src/fryvpn.rs` | FryVPN-specific implementation |
| `src/meta.rs` | UI metadata structures |
| `INTEGRATION.md` | Steps to add to FEM repo |
| `README.md` | This file |

## Dependencies

- **serde** — serialization (for metadata)
- **serde_json** — JSON parsing (health responses)
- **ureq** — blocking HTTP client (health checks)
- **thiserror** — ergonomic error types

All dependencies are production-ready and widely used.

## Status

✓ Full implementation (no TODOs/stubs)
✓ All 13 unit tests passing
✓ Clippy clean (zero warnings)
✓ Ready to integrate into FEM
