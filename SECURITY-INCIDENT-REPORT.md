# Security Incident Report: Abuse Complaint Analysis

**Date:** 2025-12-20
**Incident Type:** VPN Infrastructure Abuse - TCP SYN Flood & UDP Scan Attacks
**Severity:** CRITICAL

---

## Executive Summary

This dVPN server infrastructure has been identified as the source of network attacks including **TCP SYN floods** and **UDP scans**. After thorough analysis, the codebase **does NOT contain direct attack code**. Instead, the server is being **weaponized by external attackers** who use it as anonymizing VPN infrastructure to launch attacks.

---

## Abuse Complaint Details

| Metric | Value |
|--------|-------|
| Time Range | 2025-07-07 19:51:00 - 2025-11-18 02:42:53 (UTC+9) |
| Total Attack Count | 262,065 |
| Attacker IPs | 3 |
| Target IPs | 52 |
| Attack Types | tcp_syn_flood, udp_scan |

### Reported Attacker IPs (VPN Exit Points)

| IP Address | Location | Attacks | Type | Targets |
|------------|----------|---------|------|---------|
| 173.234.31.122 | United States | 111,758 | tcp_syn_flood | 2 IPs |
| 173.234.26.170 | United States | 88,348 | tcp_syn_flood | 1 IP |
| 64.74.163.107 | United States | 61,959 | udp_scan | 50 IPs |

---

## Root Cause Analysis

### Finding 1: Concentrated VPN Connections from 2 IPs

Analysis of `peer_usage_log.json` reveals that **76 active VPN peers** are connecting from only **2 unique IP addresses**:

| Source IP | Connected Peers | Suspicion Level |
|-----------|-----------------|-----------------|
| 175.107.227.133 | 44 connections | **CRITICAL** |
| 137.59.147.2 | 32 connections | **CRITICAL** |

**Why This Is Suspicious:**
- Legitimate VPN users connect from diverse IPs
- 76 peers from 2 IPs indicates orchestrated, automated usage
- This pattern is consistent with **botnet infrastructure** or **DDoS-as-a-Service platforms**

### Finding 2: Unrestricted Network Access Configuration

All VPN peers are configured with:

```
AllowedIPs = 0.0.0.0/0,::0/0
```

This allows **full-tunnel routing**, meaning VPN clients can route ANY traffic (including attack traffic) through this server. The traffic then exits from the server's assigned IPs (the ones reported in the abuse complaint).

### Finding 3: No Authentication on Peer Creation API

The `/create-peer` API endpoint (`main.py:114`) accepts any wallet address without:
- Rate limiting
- IP-based restrictions
- Strong authentication
- Abuse prevention measures

This enables automated mass provisioning of VPN peers for attack infrastructure.

### Finding 4: Weak wgrest Authentication

The WireGuard REST interface uses a hardcoded token:
```bash
wgrest --static-auth-token "secret" --listen "0.0.0.0:80"
```

---

## Attack Flow Diagram

```
Attacker Machines              dVPN Server                  Victims
(137.59.147.2)       ───▶     (This Server)       ───▶    (Target IPs)
(175.107.227.133)              │                           154.19.184.154:80
      │                        │                           154.19.184.112:8080
      │                        ▼
      │                   Traffic exits via:
      │                   - 173.234.31.122
      │                   - 173.234.26.170
      │                   - 64.74.163.107
      │
      └──── 76 VPN connections with different keys
           (each with full 0.0.0.0/0 routing)
```

---

## Files Analyzed

### Python Scripts (No Direct Attack Code Found)

| File | Purpose | Status |
|------|---------|--------|
| `main.py` | FastAPI REST API for peer management | Clean - but lacks security controls |
| `peer_tracker.py` | Monitors WireGuard peer connections | Clean |
| `create_wg_peer.py` | Peer creation utility | Clean |
| `create_wg_peer1.py` | Alternative peer creation | Clean |
| `generate_keys.py` | Key generation utility | Clean |

### Shell Scripts

| File | Purpose | Status |
|------|---------|--------|
| `start-wgrest.sh` | Starts wgrest daemon | Weak auth token |
| `register_peer.sh` | Registers peers with wgrest | Clean |

### Data Files

| File | Contents | Risk |
|------|----------|------|
| `ip_pool.json` | 393 peer configurations with private keys | Contains credentials |
| `peer_usage_log.json` | 76 active connections from 2 IPs | Evidence of abuse |
| `v1/*.conf` | 131 WireGuard config files | Contains credentials |

---

## Evidence Summary

### Connection Pattern Analysis

From `peer_usage_log.json`:
- **Total Active Peers:** 76
- **Unique Source IPs:** 2 (137.59.147.2 and 175.107.227.133)
- **All peers configured with:** `AllowedIPs = 0.0.0.0/0,::0/0`
- **Data transferred:** Hundreds of MiB per peer

### Timeline Correlation

- **Server Started:** 2025-06-28 20:45:45 UTC
- **Attack Period:** 2025-07-07 to 2025-11-18
- **Overlap:** Server was operational during entire attack period

---

## Immediate Remediation Steps

### 1. Block Malicious Source IPs

Add firewall rules to block the attacking client IPs:
```bash
iptables -A INPUT -s 137.59.147.2 -j DROP
iptables -A INPUT -s 175.107.227.133 -j DROP
```

### 2. Revoke All Existing Peers

Delete all peer configurations to stop ongoing attacks:
```bash
# Via API
curl -X DELETE http://localhost:8000/peers/delete-all
```

### 3. Restrict API Access

- Add authentication to `/create-peer` endpoint
- Implement rate limiting
- Add IP allowlisting for administrative access

### 4. Restrict VPN Routing

Change `AllowedIPs` from `0.0.0.0/0` to specific ranges to prevent arbitrary traffic routing.

### 5. Improve wgrest Security

Replace the hardcoded `"secret"` token with a strong, randomly generated token.

---

## Long-Term Recommendations

1. **Implement Rate Limiting:** Limit peer creation per wallet/IP
2. **Add Abuse Detection:** Monitor for high-traffic peers and unusual patterns
3. **Restrict Allowed IPs:** Don't allow full-tunnel (0.0.0.0/0) access
4. **Logging & Monitoring:** Implement traffic logging for forensics
5. **KYC/Identity Verification:** Consider requiring identity verification for VPN access
6. **Egress Filtering:** Block known attack traffic patterns at the firewall level

---

## Conclusion

The dVPN server codebase does not contain direct attack code. However, the infrastructure is being actively exploited by attackers who:

1. Create VPN peers through the unauthenticated API
2. Connect from a small number of orchestrator IPs (137.59.147.2, 175.107.227.133)
3. Route attack traffic (TCP SYN floods, UDP scans) through the VPN
4. The attack traffic exits from the server's assigned IPs (173.234.31.122, 173.234.26.170, 64.74.163.107)

**The server must be immediately secured or taken offline to prevent further abuse.**

---

## Implemented Security Fixes

The following security measures have been implemented in this PR:

### 1. API Authentication (main.py)

All API endpoints now require Bearer token authentication:
```bash
# Set environment variable before starting server
export DVPN_API_TOKEN="your-secure-token-here"

# API calls now require Authorization header
curl -H "Authorization: Bearer $DVPN_API_TOKEN" http://localhost:8000/create-peer
```

### 2. Rate Limiting (main.py)

- **10 requests per hour** per IP address (configurable via `RATE_LIMIT_REQUESTS`)
- **Maximum 5 peers per IP** to prevent mass provisioning
- Returns HTTP 429 when limits exceeded

### 3. IP Blocking (main.py + blocked_ips.json)

- Malicious IPs hardcoded in application: `137.59.147.2`, `175.107.227.133`
- Additional IPs can be added via `/admin/block-ip` endpoint
- Blocked IPs stored in `blocked_ips.json`

### 4. Secure wgrest Token (start-wgrest.sh)

- Token now loaded from `WGREST_AUTH_TOKEN` environment variable
- Script refuses to start with weak "secret" token
- Auto-generates secure token if not provided

### 5. Abuse Detection Script (scripts/detect-abuse.py)

- Analyzes peer connections for suspicious patterns
- Alerts when single IP has many VPN connections
- Identifies high-traffic peers
- Can be run via cron for continuous monitoring

### 6. Firewall Script (scripts/block-malicious-ips.sh)

- Adds iptables rules to block identified attack sources
- Includes instructions for persistence across reboots

### 7. Admin Endpoints

New authenticated admin endpoints:
- `POST /admin/block-ip?ip=x.x.x.x` - Block an IP
- `POST /admin/unblock-ip?ip=x.x.x.x` - Unblock an IP
- `GET /admin/blocked-ips` - List blocked IPs
- `GET /admin/security-status` - View security configuration

---

## Post-Deployment Checklist

After merging this PR, complete these steps:

1. **Generate secure tokens:**
   ```bash
   export DVPN_API_TOKEN=$(openssl rand -base64 32)
   export WGREST_AUTH_TOKEN=$(openssl rand -base64 32)
   ```

2. **Run firewall blocking script:**
   ```bash
   sudo ./scripts/block-malicious-ips.sh
   ```

3. **Delete all existing peers (they may be compromised):**
   ```bash
   curl -X DELETE -H "Authorization: Bearer $DVPN_API_TOKEN" \
     http://localhost:8000/peers/delete-all
   ```

4. **Restart services with new configuration**

5. **Set up abuse detection cron job:**
   ```bash
   # Add to crontab -e
   0 * * * * /path/to/scripts/detect-abuse.py >> /var/log/dvpn-abuse.log 2>&1
   ```

6. **Respond to hosting provider** with action plan

---

## Appendix: Key Files with Sensitive Data

The following files contain private keys and should be rotated or deleted:

- `ip_pool.json` - Contains all peer private keys and configurations
- `client_private.key` - Client private key
- `client_preshared.key` - Preshared key
- `v1/*.conf` - 131 configuration files with private keys
