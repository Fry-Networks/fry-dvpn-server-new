# Fry dVPN — Mainnet Deployment

Deployed to Algorand **mainnet** by the Fry Treasury. Both apps' `admin` (and creator)
is the treasury wallet `E2F2LT2INE75DBOYHQXTCTOP2PAP5MHAXQRXTTCCXFKHQTVG36DJONBQZE`.

| Contract | Mainnet App ID | App address | Admin |
|---|---|---|---|
| NodeRegistry | `3636586918` | `IVSVIHEWZ7OWF2PZSAZRUWM24EE2KTNPCD32F5VWJH5X3QKAPYDNRFDXTM` | E2F2LT2… (treasury) |
| RewardPool | `3636586967` | `P6HXMTLJWUC3M336UNTYAVABX7FQKKZLMVCCAYCCY6BCALLSZKSNV75RTU` | E2F2LT2… (treasury) |

fVPN ASA: `2485198745` — RewardPool is opted in (holds 1.1 ALGO for MBR + inner fees).

## Wiring
- **Node daemon** (`node/`): run with `REGISTRY_APP_ID=3636586918` and `FVPN_ASA_ID=2485198745`
  against a mainnet algod (e.g. `ALGOD_SERVER=https://mainnet-api.algonode.cloud ALGOD_PORT=443`).
- **Client**: `VITE_REGISTRY_APP_ID=3636586918`, `VITE_FVPN_ASA_ID=2485198745`.

## Notes
- Deployed July 2026 via `contracts/scripts/deploy.py` (`DVPN_NETWORK=mainnet`).
- Apps are **permanent** — the contracts have no `DeleteApplication` handler.
- Admin is fixed to the creator (treasury) and **cannot be changed** — the contracts
  have no `transfer_admin` method; deploying from the treasury account is what makes the
  treasury the admin.

## Deferred
- Fund the RewardPool app with fVPN so it can distribute node rewards (only ALGO + opt-in
  are configured by the deploy).
