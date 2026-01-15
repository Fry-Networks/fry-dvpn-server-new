#!/bin/bash
#
# Block Malicious IPs - Immediate Response Script
# Run this script with sudo to block IPs identified in the abuse report
#

set -e

echo "=== dVPN Security: Blocking Malicious IPs ==="
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo ""

# IPs identified from peer_usage_log.json analysis
# These IPs had 76 connections from just 2 sources (botnet pattern)
MALICIOUS_IPS=(
    "137.59.147.2"      # 32 suspicious VPN connections
    "175.107.227.133"   # 44 suspicious VPN connections
)

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root (sudo)"
    exit 1
fi

# Check if iptables is available
if ! command -v iptables &> /dev/null; then
    echo "ERROR: iptables not found"
    exit 1
fi

echo "Blocking ${#MALICIOUS_IPS[@]} malicious IP addresses..."
echo ""

for ip in "${MALICIOUS_IPS[@]}"; do
    # Check if rule already exists
    if iptables -C INPUT -s "$ip" -j DROP 2>/dev/null; then
        echo "[SKIP] $ip is already blocked"
    else
        iptables -A INPUT -s "$ip" -j DROP
        echo "[BLOCKED] $ip"
    fi
done

echo ""
echo "=== Current iptables DROP rules ==="
iptables -L INPUT -n | grep DROP || echo "No DROP rules found"

echo ""
echo "=== Blocking Complete ==="
echo ""
echo "To make these rules persistent across reboots:"
echo "  Ubuntu/Debian: sudo apt install iptables-persistent && sudo netfilter-persistent save"
echo "  RHEL/CentOS:   sudo service iptables save"
echo ""
echo "To unblock an IP:"
echo "  sudo iptables -D INPUT -s <IP> -j DROP"
