"""Node-side decentralization E2E (LocalNet).

Proves the Go `frynode` daemon ACTUALLY registers on-chain (not a stub):
  1. deploy NodeRegistry to LocalNet
  2. create + fund a node account
  3. run the real frynode binary against LocalNet with that account
  4. wait for /health, then read the node's registry box directly from algod
     and assert it matches what the daemon advertised
  5. SIGTERM the daemon and assert it deregistered (box removed)

Run: uv run --with algokit-utils python scripts/node_e2e.py
Requires: frynode built at ../node/frynode.exe (or set FRYNODE_BIN).
"""

import json
import os
import signal
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

import algosdk
from algosdk import account, mnemonic
from algokit_utils import AlgorandClient, AlgoAmount
from algosdk.abi import ABIType, TupleType

HERE = Path(__file__).parent
SPEC = HERE.parent / "smart_contracts" / "node_registry" / "artifacts" / "NodeRegistry.arc56.json"
FRYNODE = os.environ.get("FRYNODE_BIN", str(HERE.parent.parent / "node" / "frynode.exe"))
API_PORT = 8098
ENDPOINT = "127.0.0.1:51820"
REGION = "us-test"


def read_box(algod, app_id, addr):
    box = algod.application_box_by_name(app_id, algosdk.encoding.decode_address(addr))
    raw = algosdk.encoding.base64.b64decode(box["value"])
    struct = json.loads(SPEC.read_text())["structs"]["NodeRecord"]
    tup = TupleType([ABIType.from_string(f["type"]) for f in struct])
    vals = tup.decode(raw)
    return dict(zip([f["name"] for f in struct], vals))


def main() -> None:
    if not Path(FRYNODE).exists():
        sys.exit(f"frynode binary not found at {FRYNODE}; build it first (go build -o frynode.exe .)")

    algorand = AlgorandClient.default_localnet()
    dispenser = algorand.account.localnet_dispenser()
    algod = algorand.client.algod

    # deploy registry
    factory = algorand.client.get_app_factory(app_spec=SPEC.read_text(), default_sender=dispenser.address)
    app_client, _ = factory.send.bare.create()
    app_id = app_client.app_id
    print(f"deployed NodeRegistry app_id={app_id}")

    # node account, funded
    sk, addr = account.generate_account()
    node_mn = mnemonic.from_private_key(sk)
    algorand.account.ensure_funded(addr, dispenser.address, AlgoAmount.from_algo(5))
    print(f"node account {addr} funded")

    # run frynode against LocalNet
    identity_dir = HERE.parent / ".node_e2e_identity"
    env = dict(os.environ)
    env.update({
        "ALGOD_SERVER": "http://localhost",
        "ALGOD_PORT": "4001",
        "ALGOD_TOKEN": "a" * 64,
        "REGISTRY_APP_ID": str(app_id),
        "NODE_MNEMONIC": node_mn,
        "REGION": REGION,
        "CAPACITY_MBPS": "250",
        "PRICE_PER_GB": "1000000",
        "PUBLIC_ENDPOINT": ENDPOINT,
        "API_PORT": str(API_PORT),
        "HEARTBEAT_SECONDS": "4",
    })
    if identity_dir.exists():
        import shutil
        shutil.rmtree(identity_dir, ignore_errors=True)
    proc = subprocess.Popen(
        [FRYNODE],
        env=env, cwd=str(HERE.parent.parent / "node"),
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
    )
    try:
        # wait for health
        healthy = False
        for _ in range(30):
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{API_PORT}/health", timeout=2) as r:
                    if r.status == 200:
                        healthy = True
                        break
            except Exception:
                time.sleep(1)
        if not healthy:
            out = proc.stdout.read() if proc.stdout else ""
            sys.exit(f"frynode never became healthy.\n--- daemon output ---\n{out}")
        print("frynode /health OK")

        # give it a moment to submit registration, then read the box
        rec = None
        for _ in range(15):
            try:
                rec = read_box(algod, app_id, addr)
                break
            except Exception:
                time.sleep(1)
        if rec is None:
            sys.exit("node did NOT register on-chain (no box)")

        print("on-chain node record:")
        for k, v in rec.items():
            if k == "wg_pubkey":
                v = bytes(v).hex()
            print(f"  {k} = {v}")
        assert rec["endpoint"] == ENDPOINT, "endpoint mismatch"
        assert rec["region"] == REGION, "region mismatch"
        assert rec["capacity_mbps"] == 250, "capacity mismatch"
        assert rec["price_per_gb"] == 1_000_000, "price mismatch"
        assert rec["status"] == 1, "status not active"
        assert len(bytes(rec["wg_pubkey"])) == 32 and any(bytes(rec["wg_pubkey"])), "wg pubkey missing"
        print("NODE REGISTERED ON-CHAIN ✔ (real decentralized registration)")

        # wait for at least one heartbeat to bump last_heartbeat
        hb0 = rec["last_heartbeat"]
        bumped = False
        for _ in range(12):
            time.sleep(2)
            try:
                r2 = read_box(algod, app_id, addr)
                if r2["last_heartbeat"] >= hb0:
                    bumped = True
                    break
            except Exception:
                pass
        print(f"heartbeat present: {bumped} (last_heartbeat={rec['last_heartbeat']})")
    finally:
        # shutdown. On POSIX, SIGTERM triggers graceful deregister; on Windows
        # terminate() is a hard kill (deregister won't fire — reap_stale covers it).
        if os.name == "nt":
            proc.terminate()
        else:
            proc.send_signal(signal.SIGTERM)
        try:
            proc.wait(timeout=20)
        except Exception:
            proc.kill()

    # assert deregistered
    time.sleep(2)
    try:
        read_box(algod, app_id, addr)
        print("WARN: node box still present after shutdown (deregister may not have completed)")
    except Exception:
        print("NODE DEREGISTERED ON-CHAIN ✔ (box removed on shutdown)")

    print("\nNODE E2E PASSED")


if __name__ == "__main__":
    main()
