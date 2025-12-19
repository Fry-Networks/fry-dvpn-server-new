import os
import json
import subprocess
import base64
from datetime import datetime, timezone

# === CONFIG ===
WG_INTERFACE = "wg0"
PEER_LOG_FILE = "peer_usage_log.json"
IP_POOL_FILE = "ip_pool.json"

# === BASE64 Normalizer ===
def normalize_b64key(key: str) -> str:
    key = key.strip().replace('-', '+').replace('_', '/')
    key += '=' * (-len(key) % 4)
    try:
        decoded = base64.b64decode(key)
        return base64.b64encode(decoded).decode().strip()
    except Exception:
        return key

# === Load JSON file (safe) ===
def load_json(filename):
    if not os.path.exists(filename) or os.path.getsize(filename) == 0:
        return {}
    try:
        with open(filename, "r") as f:
            return json.load(f)
    except json.JSONDecodeError:
        print(f"⚠️ Warning: {filename} is not valid JSON.")
        return {}

# === Save JSON file ===
def save_json(filename, data):
    with open(filename, "w") as f:
        json.dump(data, f, indent=2)

# === Run WireGuard command ===
def run_wg_show():
    result = subprocess.run(["wg", "show", WG_INTERFACE], capture_output=True, text=True)
    return result.stdout.strip().splitlines()

# === Parse `wg show` output ===
def parse_wg_output(lines):
    peers = {}
    current_peer = None
    for line in lines:
        line = line.strip()
        if line.startswith("peer:"):
            current_peer = line.split("peer:")[1].strip()
            peers[current_peer] = {"public_key": current_peer}
        elif current_peer:
            if line.startswith("endpoint:"):
                peers[current_peer]["endpoint"] = line.split("endpoint:")[1].strip()
            elif line.startswith("allowed ips:"):
                peers[current_peer]["allowed_ips"] = line.split("allowed ips:")[1].strip()
            elif line.startswith("latest handshake:"):
                handshake = line.split("latest handshake:")[1].strip()
                peers[current_peer]["last_seen"] = datetime.now(timezone.utc).isoformat()
                peers[current_peer]["connected"] = "ago" in handshake
                peers[current_peer]["disconnected"] = not peers[current_peer]["connected"]
            elif line.startswith("transfer:"):
                txrx = line.split("transfer:")[1].strip().split(",")
                peers[current_peer]["rx"] = txrx[0].strip()
                peers[current_peer]["tx"] = txrx[1].strip()
    return peers

# === Build wallet address mapping ===
def get_wallet_mapping():
    pool = load_json(IP_POOL_FILE)
    mapping = {}
    for entry in pool.values():
        pub_key = entry.get("public_key")
        wallet = entry.get("wallet_address")
        if pub_key and wallet and wallet != "unknown":
            normalized = normalize_b64key(pub_key)
            mapping[normalized] = wallet
    return mapping

# === Update the peer log ===
def update_peer_log():
    print("🔄 Updating peer usage log...")
    lines = run_wg_show()
    peer_data = parse_wg_output(lines)
    peer_log = {}
    wallet_map = get_wallet_mapping()

    for raw_pub_key, info in peer_data.items():
        norm_pub_key = normalize_b64key(raw_pub_key)
        wallet_address = wallet_map.get(norm_pub_key, "unknown")

        print(f"🔍 Peer: {raw_pub_key}")
        print(f"   ↳ Wallet: {wallet_address}")

        info["wallet_address"] = wallet_address
        info["connect_time"] = datetime.now(timezone.utc).isoformat()
        peer_log[norm_pub_key] = info

    save_json(PEER_LOG_FILE, peer_log)
    print("✅ Peer usage log updated.")

# === Entry point ===
if __name__ == "__main__":
    update_peer_log()
