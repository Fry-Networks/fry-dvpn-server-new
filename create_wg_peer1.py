import subprocess
import json

# === CONFIG ===
CLIENT_IP = "10.66.66.2/32"
CLIENT_IPv6 = "fd42:42:42::2/128"
DNS = "8.8.8.8,1.1.1.1,2001:4860:4860::8888,2606:4700:4700::1111"
ENDPOINT = "54.211.138.164:54331"
SERVER_PUBLIC_KEY = "47uIyx2/jnb56lSpgY+mOGCZudmbjEDb7N8YU+TviVk="

# === GENERATE KEYS ===
def generate_key(command):
    result = subprocess.run(command, capture_output=True, text=True, check=True)
    return result.stdout.strip()

print("🔐 Generating client keys...")

private_key = generate_key(["wg", "genkey"])
public_key = subprocess.run(["wg", "pubkey"], input=private_key, capture_output=True, text=True, check=True).stdout.strip()
preshared_key = generate_key(["wg", "genpsk"])

print("🔐 Generated Keys:")
print(f"Private Key   : {private_key}")
print(f"Public Key    : {public_key}")
print(f"Preshared Key : {preshared_key}")

# === BUILD CONF STRING ===
conf = f"""[Interface]
PrivateKey = {private_key}
Address = {CLIENT_IP},{CLIENT_IPv6}
DNS = {DNS}

[Peer]
PublicKey = {SERVER_PUBLIC_KEY}
PresharedKey = {preshared_key}
Endpoint = {ENDPOINT}
AllowedIPs = 0.0.0.0/0,::0/0
"""

# === BUILD JSON PAYLOAD ===
json_output = {
    "public_key": public_key,
    "conf": conf
}

print("\n✅ Output JSON:")
print(json.dumps(json_output, indent=2))
