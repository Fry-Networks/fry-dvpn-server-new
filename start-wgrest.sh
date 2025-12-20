#!/bin/bash

# Load auth token from environment variable
# Generate a secure token if not set: openssl rand -base64 32
WGREST_AUTH_TOKEN="${WGREST_AUTH_TOKEN:-$(openssl rand -base64 32)}"

if [ -z "$WGREST_AUTH_TOKEN" ] || [ "$WGREST_AUTH_TOKEN" == "secret" ]; then
    echo "ERROR: WGREST_AUTH_TOKEN must be set to a secure value"
    echo "Generate one with: openssl rand -base64 32"
    exit 1
fi

echo "Starting wgrest with secure token..."
wgrest --static-auth-token "$WGREST_AUTH_TOKEN" --listen "0.0.0.0:80"
