from fastapi import FastAPI, HTTPException, Query
from pydantic import BaseModel
import base64, json, os, requests
from nacl.public import PrivateKey
import hashlib

# === CONFIG ===
WGREST_API = "http://localhost/v1/devices/wg0/peers/"
AUTH_TOKEN = "secret"
PERSISTENT_KEEPALIVE = 25
IP_POOL_FILE = "ip_pool.json"
BASE_IPV4 = "10.66.66."
BASE_IPV6 = "fd42:42:42::"
START = 2
END = 254

app = FastAPI()

class PeerRequest(BaseModel):
    wallet_address: str

# === UTILS ===
def wallet_key(wallet_address):
    return hashlib.sha256(wallet_address.encode()).hexdigest()

def load_ip_pool():
    if not os.path.exists(IP_POOL_FILE):
        return {}
    with open(IP_POOL_FILE, "r") as f:
        return json.load(f)

def save_ip_pool(pool):
    with open(IP_POOL_FILE, "w") as f:
        json.dump(pool, f, indent=2)

def allocate_ip(wallet_key):
    pool = load_ip_pool()
    used_ips = set(entry["ipv4"] for entry in pool.values())
    for i in range(START, END + 1):
        ipv4 = f"{BASE_IPV4}{i}/32"
        if ipv4 not in used_ips:
            ipv6 = f"{BASE_IPV6}{i}/128"
            return ipv4, ipv6
    raise Exception("❌ No available IPs in pool")

def generate_keys():
    private_key = PrivateKey.generate()
    public_key = private_key.public_key
    preshared_key = os.urandom(32)

    return (
        base64.b64encode(bytes(private_key)).decode(),
        base64.b64encode(bytes(public_key)).decode(),
        base64.b64encode(preshared_key).decode()
    )

# === CORE LOGIC ===
def create_peer(wallet_address):
    wkey = wallet_key(wallet_address)
    pool = load_ip_pool()

    if wkey in pool:
        return {
            "peer_name": wallet_address,
            "public_key": pool[wkey]["public_key"],
            "ipv4": pool[wkey]["ipv4"],
            "ipv6": pool[wkey]["ipv6"],
            "conf": pool[wkey]["conf"]
        }

    ipv4, ipv6 = allocate_ip(wkey)
    private_key_b64, public_key_b64, preshared_key_b64 = generate_keys()

    payload = {
        "preshared_key": preshared_key_b64,
        "allowed_ips": [ipv4, ipv6],
        "persistent_keepalive": PERSISTENT_KEEPALIVE
    }

    headers = {
        "Authorization": f"Bearer {AUTH_TOKEN}",
        "Content-Type": "application/json"
    }

    response = requests.post(WGREST_API, headers=headers, data=json.dumps(payload))
    if response.status_code != 201:
        raise HTTPException(status_code=500, detail=f"Failed to register peer: {response.text}")

    resp_data = response.json()
    safe_key = resp_data["url_safe_public_key"]
    conf_url = WGREST_API + safe_key + "/quick.conf"
    conf_response = requests.get(conf_url, headers=headers)

    if conf_response.status_code == 200 and "[Interface]" in conf_response.text:
        pool[wkey] = {
            "wallet_address": wallet_address,
            "ipv4": ipv4,
            "ipv6": ipv6,
            "public_key": safe_key,  # ✅ Store correct WireGuard public key
            "conf": conf_response.text
        }
        save_ip_pool(pool)
        return {
            "peer_name": wallet_address,
            "public_key": safe_key,
            "ipv4": ipv4,
            "ipv6": ipv6,
            "conf": conf_response.text
        }

    raise HTTPException(status_code=500, detail=f"Failed to fetch config: {conf_response.text}")

# === API ROUTES ===
@app.post("/create-peer")
def create_peer_api(data: PeerRequest):
    return create_peer(data.wallet_address)

@app.get("/peer-usage")
def get_peer_usage():
    if not os.path.exists("peer_usage_log.json"):
        raise HTTPException(status_code=404, detail="Peer usage log not found")
    with open("peer_usage_log.json", "r") as f:
        return json.load(f)

@app.get("/peer-usage/by-wallet")
def get_peer_by_wallet(wallet_address: str = Query(...)):
    if not os.path.exists("peer_usage_log.json"):
        raise HTTPException(status_code=404, detail="Log not found")
    with open("peer_usage_log.json", "r") as f:
        data = json.load(f)
        for peer in data.values():
            if peer.get("wallet_address") == wallet_address:
                return peer
    raise HTTPException(status_code=404, detail="Wallet address not found")

@app.delete("/peers/delete-all")
def delete_all_peers():
    headers = {"Authorization": f"Bearer {AUTH_TOKEN}"}
    try:
        response = requests.get(WGREST_API, headers=headers)
        response.raise_for_status()
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to list peers: {str(e)}")

    peers = response.json()
    deleted, errors = [], []

    for peer in peers:
        key = peer.get("url_safe_public_key")
        if not key:
            continue
        try:
            del_resp = requests.delete(f"{WGREST_API}{key}", headers=headers)
            if del_resp.status_code == 204:
                deleted.append(key)
            else:
                errors.append({key: del_resp.text})
        except Exception as e:
            errors.append({key: str(e)})

    if os.path.exists(IP_POOL_FILE):
        os.remove(IP_POOL_FILE)

    return {
        "deleted": deleted,
        "errors": errors,
        "summary": f"{len(deleted)} peers deleted, {len(errors)} errors",
        "ip_pool_cleared": True
    }
