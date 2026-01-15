from fastapi import FastAPI, HTTPException, Query, Request, Depends
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
from pydantic import BaseModel
import base64, json, os, requests, time, secrets
from nacl.public import PrivateKey
import hashlib
from collections import defaultdict
from datetime import datetime, timezone

# === CONFIG ===
WGREST_API = "http://localhost/v1/devices/wg0/peers/"
# Use environment variable for wgrest token, with secure default generation
WGREST_AUTH_TOKEN = os.environ.get("WGREST_AUTH_TOKEN", secrets.token_urlsafe(32))
# API authentication token - MUST be set via environment variable
API_AUTH_TOKEN = os.environ.get("DVPN_API_TOKEN")
PERSISTENT_KEEPALIVE = 25
IP_POOL_FILE = "ip_pool.json"
BLOCKED_IPS_FILE = "blocked_ips.json"
BASE_IPV4 = "10.66.66."
BASE_IPV6 = "fd42:42:42::"
START = 2
END = 254

# === RATE LIMITING CONFIG ===
RATE_LIMIT_REQUESTS = int(os.environ.get("RATE_LIMIT_REQUESTS", "10"))  # requests per window
RATE_LIMIT_WINDOW = int(os.environ.get("RATE_LIMIT_WINDOW", "3600"))  # window in seconds (1 hour)
MAX_PEERS_PER_IP = int(os.environ.get("MAX_PEERS_PER_IP", "5"))  # max peers from single IP

# === KNOWN MALICIOUS IPS (from abuse report) ===
DEFAULT_BLOCKED_IPS = [
    "137.59.147.2",      # 32 suspicious VPN connections
    "175.107.227.133",   # 44 suspicious VPN connections
]

app = FastAPI(title="Secure dVPN Server", version="2.0.0")
security = HTTPBearer()

# Rate limiting storage (in production, use Redis)
rate_limit_store = defaultdict(list)
ip_peer_count = defaultdict(int)

class PeerRequest(BaseModel):
    wallet_address: str

# === SECURITY FUNCTIONS ===
def load_blocked_ips():
    """Load blocked IPs from file and merge with defaults"""
    blocked = set(DEFAULT_BLOCKED_IPS)
    if os.path.exists(BLOCKED_IPS_FILE):
        try:
            with open(BLOCKED_IPS_FILE, "r") as f:
                data = json.load(f)
                blocked.update(data.get("blocked_ips", []))
        except (json.JSONDecodeError, IOError):
            pass
    return blocked

def save_blocked_ips(ips: list):
    """Save blocked IPs to file"""
    with open(BLOCKED_IPS_FILE, "w") as f:
        json.dump({"blocked_ips": list(ips), "updated": datetime.now(timezone.utc).isoformat()}, f, indent=2)

def get_client_ip(request: Request) -> str:
    """Extract client IP from request, handling proxies"""
    forwarded = request.headers.get("X-Forwarded-For")
    if forwarded:
        return forwarded.split(",")[0].strip()
    return request.client.host if request.client else "unknown"

def is_ip_blocked(ip: str) -> bool:
    """Check if IP is in blocked list"""
    blocked_ips = load_blocked_ips()
    return ip in blocked_ips

def check_rate_limit(ip: str) -> bool:
    """Check if IP has exceeded rate limit. Returns True if allowed."""
    now = time.time()
    window_start = now - RATE_LIMIT_WINDOW

    # Clean old entries
    rate_limit_store[ip] = [t for t in rate_limit_store[ip] if t > window_start]

    if len(rate_limit_store[ip]) >= RATE_LIMIT_REQUESTS:
        return False

    rate_limit_store[ip].append(now)
    return True

def verify_api_token(credentials: HTTPAuthorizationCredentials = Depends(security)) -> bool:
    """Verify API authentication token"""
    if not API_AUTH_TOKEN:
        raise HTTPException(
            status_code=500,
            detail="API authentication not configured. Set DVPN_API_TOKEN environment variable."
        )
    if not secrets.compare_digest(credentials.credentials, API_AUTH_TOKEN):
        raise HTTPException(status_code=401, detail="Invalid authentication token")
    return True

def security_check(request: Request):
    """Combined security checks: IP blocking and rate limiting"""
    client_ip = get_client_ip(request)

    # Check if IP is blocked
    if is_ip_blocked(client_ip):
        raise HTTPException(
            status_code=403,
            detail=f"Access denied: IP {client_ip} is blocked due to abuse"
        )

    # Check rate limit
    if not check_rate_limit(client_ip):
        raise HTTPException(
            status_code=429,
            detail=f"Rate limit exceeded. Maximum {RATE_LIMIT_REQUESTS} requests per {RATE_LIMIT_WINDOW} seconds."
        )

    return client_ip

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
    raise Exception("No available IPs in pool")

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
def create_peer(wallet_address: str, client_ip: str):
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

    # Check max peers per IP
    if ip_peer_count[client_ip] >= MAX_PEERS_PER_IP:
        raise HTTPException(
            status_code=429,
            detail=f"Maximum peers per IP ({MAX_PEERS_PER_IP}) exceeded"
        )

    ipv4, ipv6 = allocate_ip(wkey)
    private_key_b64, public_key_b64, preshared_key_b64 = generate_keys()

    payload = {
        "preshared_key": preshared_key_b64,
        "allowed_ips": [ipv4, ipv6],
        "persistent_keepalive": PERSISTENT_KEEPALIVE
    }

    headers = {
        "Authorization": f"Bearer {WGREST_AUTH_TOKEN}",
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
            "public_key": safe_key,
            "conf": conf_response.text,
            "created_at": datetime.now(timezone.utc).isoformat(),
            "created_by_ip": client_ip
        }
        save_ip_pool(pool)
        ip_peer_count[client_ip] += 1

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
def create_peer_api(
    data: PeerRequest,
    request: Request,
    authenticated: bool = Depends(verify_api_token)
):
    """Create a new VPN peer. Requires authentication."""
    client_ip = security_check(request)
    return create_peer(data.wallet_address, client_ip)

@app.get("/peer-usage")
def get_peer_usage(authenticated: bool = Depends(verify_api_token)):
    """Get peer usage statistics. Requires authentication."""
    if not os.path.exists("peer_usage_log.json"):
        raise HTTPException(status_code=404, detail="Peer usage log not found")
    with open("peer_usage_log.json", "r") as f:
        return json.load(f)

@app.get("/peer-usage/by-wallet")
def get_peer_by_wallet(
    wallet_address: str = Query(...),
    authenticated: bool = Depends(verify_api_token)
):
    """Get peer usage by wallet address. Requires authentication."""
    if not os.path.exists("peer_usage_log.json"):
        raise HTTPException(status_code=404, detail="Log not found")
    with open("peer_usage_log.json", "r") as f:
        data = json.load(f)
        for peer in data.values():
            if peer.get("wallet_address") == wallet_address:
                return peer
    raise HTTPException(status_code=404, detail="Wallet address not found")

@app.delete("/peers/delete-all")
def delete_all_peers(authenticated: bool = Depends(verify_api_token)):
    """Delete all VPN peers. Requires authentication."""
    headers = {"Authorization": f"Bearer {WGREST_AUTH_TOKEN}"}
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

    # Reset peer count
    ip_peer_count.clear()

    return {
        "deleted": deleted,
        "errors": errors,
        "summary": f"{len(deleted)} peers deleted, {len(errors)} errors",
        "ip_pool_cleared": True
    }

# === ADMIN ROUTES ===

@app.post("/admin/block-ip")
def block_ip(
    ip: str = Query(..., description="IP address to block"),
    authenticated: bool = Depends(verify_api_token)
):
    """Block an IP address from accessing the VPN service."""
    blocked = load_blocked_ips()
    blocked.add(ip)
    save_blocked_ips(list(blocked))
    return {"message": f"IP {ip} has been blocked", "total_blocked": len(blocked)}

@app.post("/admin/unblock-ip")
def unblock_ip(
    ip: str = Query(..., description="IP address to unblock"),
    authenticated: bool = Depends(verify_api_token)
):
    """Unblock an IP address."""
    blocked = load_blocked_ips()
    if ip in DEFAULT_BLOCKED_IPS:
        return {"error": f"Cannot unblock {ip} - it's in the permanent block list due to abuse"}
    blocked.discard(ip)
    save_blocked_ips(list(blocked))
    return {"message": f"IP {ip} has been unblocked", "total_blocked": len(blocked)}

@app.get("/admin/blocked-ips")
def list_blocked_ips(authenticated: bool = Depends(verify_api_token)):
    """List all blocked IP addresses."""
    blocked = load_blocked_ips()
    return {
        "blocked_ips": list(blocked),
        "permanent_blocks": DEFAULT_BLOCKED_IPS,
        "total": len(blocked)
    }

@app.get("/admin/security-status")
def security_status(authenticated: bool = Depends(verify_api_token)):
    """Get security status and configuration."""
    return {
        "rate_limit_requests": RATE_LIMIT_REQUESTS,
        "rate_limit_window_seconds": RATE_LIMIT_WINDOW,
        "max_peers_per_ip": MAX_PEERS_PER_IP,
        "blocked_ips_count": len(load_blocked_ips()),
        "api_auth_configured": bool(API_AUTH_TOKEN),
        "wgrest_token_from_env": "WGREST_AUTH_TOKEN" in os.environ
    }

@app.get("/health")
def health_check():
    """Health check endpoint (no auth required)."""
    return {"status": "healthy", "version": "2.0.0"}
