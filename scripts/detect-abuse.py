#!/usr/bin/env python3
"""
Abuse Detection Script for dVPN Server

Analyzes peer_usage_log.json to detect suspicious connection patterns:
- Multiple peers from a single IP (botnet pattern)
- High traffic peers (potential attack sources)
- Unusual connection patterns

Run this script periodically (e.g., via cron) to detect abuse.
"""

import json
import os
import sys
from collections import defaultdict
from datetime import datetime, timezone

# Configuration
PEER_LOG_FILE = "peer_usage_log.json"
BLOCKED_IPS_FILE = "blocked_ips.json"
MAX_PEERS_PER_IP = 5  # Alert if more peers than this from single IP
HIGH_TRAFFIC_THRESHOLD_MB = 100  # Alert if peer has transferred more than this

def parse_size(size_str: str) -> float:
    """Parse size string like '468.79 MiB received' to MB"""
    if not size_str:
        return 0.0
    parts = size_str.split()
    if len(parts) < 2:
        return 0.0
    try:
        value = float(parts[0])
        unit = parts[1].lower()
        if 'gib' in unit:
            return value * 1024
        elif 'mib' in unit:
            return value
        elif 'kib' in unit:
            return value / 1024
        return value
    except (ValueError, IndexError):
        return 0.0

def load_peer_log():
    """Load peer usage log"""
    if not os.path.exists(PEER_LOG_FILE):
        print(f"ERROR: {PEER_LOG_FILE} not found")
        sys.exit(1)

    with open(PEER_LOG_FILE, "r") as f:
        return json.load(f)

def load_blocked_ips():
    """Load currently blocked IPs"""
    if not os.path.exists(BLOCKED_IPS_FILE):
        return set()
    with open(BLOCKED_IPS_FILE, "r") as f:
        data = json.load(f)
        return set(data.get("blocked_ips", []))

def save_blocked_ips(ips: set):
    """Save blocked IPs to file"""
    with open(BLOCKED_IPS_FILE, "w") as f:
        json.dump({
            "blocked_ips": list(ips),
            "updated": datetime.now(timezone.utc).isoformat()
        }, f, indent=2)

def analyze_peers():
    """Analyze peer connections for abuse patterns"""
    peers = load_peer_log()
    blocked = load_blocked_ips()

    # Group peers by source IP
    ip_peers = defaultdict(list)
    high_traffic_peers = []

    for pub_key, peer in peers.items():
        endpoint = peer.get("endpoint", "")
        if endpoint:
            ip = endpoint.split(":")[0]
            ip_peers[ip].append({
                "public_key": pub_key,
                "wallet": peer.get("wallet_address", "unknown"),
                "rx": peer.get("rx", "0"),
                "tx": peer.get("tx", "0"),
                "connected": peer.get("connected", False)
            })

            # Check for high traffic
            rx_mb = parse_size(peer.get("rx", "0"))
            tx_mb = parse_size(peer.get("tx", "0"))
            total_mb = rx_mb + tx_mb
            if total_mb > HIGH_TRAFFIC_THRESHOLD_MB:
                high_traffic_peers.append({
                    "ip": ip,
                    "public_key": pub_key,
                    "wallet": peer.get("wallet_address", "unknown"),
                    "total_mb": total_mb
                })

    # Find suspicious IPs (many peers from single source)
    suspicious_ips = []
    for ip, peers_list in ip_peers.items():
        if len(peers_list) > MAX_PEERS_PER_IP:
            suspicious_ips.append({
                "ip": ip,
                "peer_count": len(peers_list),
                "peers": peers_list,
                "already_blocked": ip in blocked
            })

    return {
        "total_peers": len(peers),
        "unique_source_ips": len(ip_peers),
        "suspicious_ips": suspicious_ips,
        "high_traffic_peers": high_traffic_peers,
        "currently_blocked": list(blocked)
    }

def main():
    print("=" * 60)
    print("dVPN Abuse Detection Report")
    print(f"Generated: {datetime.now(timezone.utc).isoformat()}")
    print("=" * 60)
    print()

    results = analyze_peers()

    print(f"Total Peers: {results['total_peers']}")
    print(f"Unique Source IPs: {results['unique_source_ips']}")
    print(f"Currently Blocked IPs: {len(results['currently_blocked'])}")
    print()

    # Report suspicious IPs
    if results['suspicious_ips']:
        print("=" * 60)
        print("ALERT: SUSPICIOUS IP ADDRESSES DETECTED")
        print(f"(More than {MAX_PEERS_PER_IP} peers from single IP)")
        print("=" * 60)

        new_threats = []
        for entry in sorted(results['suspicious_ips'], key=lambda x: x['peer_count'], reverse=True):
            status = "[ALREADY BLOCKED]" if entry['already_blocked'] else "[NEW THREAT]"
            print(f"\n{status} {entry['ip']}: {entry['peer_count']} peers")

            if not entry['already_blocked']:
                new_threats.append(entry['ip'])
                print("  Peers:")
                for peer in entry['peers'][:5]:  # Show first 5
                    print(f"    - Wallet: {peer['wallet'][:20]}... | RX: {peer['rx']} | TX: {peer['tx']}")
                if len(entry['peers']) > 5:
                    print(f"    ... and {len(entry['peers']) - 5} more")

        if new_threats:
            print()
            print("=" * 60)
            print("RECOMMENDED ACTION: Block these IPs")
            print("=" * 60)
            for ip in new_threats:
                print(f"  sudo iptables -A INPUT -s {ip} -j DROP")

            # Ask to auto-block
            if sys.stdin.isatty():
                response = input("\nAdd these IPs to blocked_ips.json? [y/N]: ")
                if response.lower() == 'y':
                    blocked = load_blocked_ips()
                    blocked.update(new_threats)
                    save_blocked_ips(blocked)
                    print(f"Added {len(new_threats)} IPs to blocked list")
    else:
        print("No suspicious IP patterns detected.")

    # Report high traffic peers
    if results['high_traffic_peers']:
        print()
        print("=" * 60)
        print(f"HIGH TRAFFIC PEERS (>{HIGH_TRAFFIC_THRESHOLD_MB} MB)")
        print("=" * 60)

        for peer in sorted(results['high_traffic_peers'], key=lambda x: x['total_mb'], reverse=True)[:10]:
            print(f"  {peer['ip']}: {peer['total_mb']:.1f} MB | Wallet: {peer['wallet'][:30]}...")

    print()
    print("=" * 60)
    print("Report Complete")
    print("=" * 60)

    # Exit with error code if new threats found
    if results['suspicious_ips'] and any(not s['already_blocked'] for s in results['suspicious_ips']):
        sys.exit(1)
    sys.exit(0)

if __name__ == "__main__":
    main()
