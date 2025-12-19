#!/bin/bash

# === CONFIG ===
WGREST_API="http://localhost/v1/devices/wg0/peers"
AUTH_TOKEN="secret"
CLIENT_IP="10.66.66.2/32"
CLIENT_IPv6="fd42:42:42::2/128"
SAVE_DIR="/var/python"

# === Read keys ===
PRIVATE_KEY=$(cat client_private.key)
PUBLIC_KEY=$(cat client_public.key)
PRESHARED_KEY=$(cat client_preshared.key)

echo "?? Using manually generated keys..."
echo "Public Key    : $PUBLIC_KEY"
echo "Preshared Key : $PRESHARED_KEY"

# === Build JSON payload ===
JSON=$(jq -n \
  --arg private_key "$PRIVATE_KEY" \
  --arg public_key "$PUBLIC_KEY" \
  --arg preshared_key "$PRESHARED_KEY" \
  --argjson allowed_ips "[\"$CLIENT_IP\", \"$CLIENT_IPv6\"]" \
  --argjson keepalive 25 \
  '{
    private_key: $private_key,
    public_key: $public_key,
    preshared_key: $preshared_key,
    allowed_ips: $allowed_ips,
    persistent_keepalive: $keepalive
  }')

# === Send peer to wgrest ===
echo -e "\n?? Registering peer with wgrest API..."
response=$(curl -s -w "\n%{http_code}" -X POST "$WGREST_API/" \
  -H "Authorization: Bearer $AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$JSON")

body=$(echo "$response" | head -n -1)
status=$(echo "$response" | tail -n 1)

if [ "$status" -eq 201 ]; then
  echo "? Peer registered."

  # Parse URL-safe public key
  SAFE_KEY=$(echo "$body" | jq -r .url_safe_public_key)
  CONF_URL="${WGREST_API}/${SAFE_KEY}/quick.conf"

  echo -e "\n?? Fetching client config from $CONF_URL..."
  config=$(curl -s -X GET "$CONF_URL" \
    -H "Authorization: Bearer $AUTH_TOKEN")

  if [[ "$config" == *"[Interface]"* ]]; then
    FILENAME="$SAVE_DIR/client-${SAFE_KEY:0:8}.conf"
    sudo mkdir -p "$SAVE_DIR"
    echo "$config" | sudo tee "$FILENAME" > /dev/null
    echo "? Config saved to: $FILENAME"
  else
    echo "? Failed to fetch config file:"
    echo "$config"
  fi
else
  echo "? API failed:"
  echo "$body"
fi
