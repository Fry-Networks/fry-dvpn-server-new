import nacl.public
import base64
import os
import requests
import json

# === CONFIG ===
WGREST_API = "http://localhost/v1/devices/wg0/peers/"
AUTH_TOKEN = "secret"
ALLOWED_IPS = ["10.66.66.2/32", "fd42:42:42::2/128"]
PERSISTENT_KEEPALIVE = 25

# === KEY GENERATION ===
private_key = nacl.public.PrivateKey.generate()
public_key = private_key.public_key
preshared_key = os.urandom(32)

private_key_b64 = base64.b64encode(bytes(private_key)).decode()
public_key_b64 = base64.b64encode(bytes(public_key)).decode()
preshared_key_b64 = base64.b64encode(preshared_key).decode()

print("🔐 Generated Keys:")
print("Private Key   :", private_key_b64)
print("Public Key    :", public_key_b64)
print("Preshared Key :", preshared_key_b64)

# === API PAYLOAD ===
payload = {
    "private_key": private_key_b64,
    "public_key": public_key_b64,
    "preshared_key": preshared_key_b64,
    "allowed_ips": ALLOWED_IPS,
    "persistent_keepalive": PERSISTENT_KEEPALIVE
}

headers = {
    "Authorization": f"Bearer {AUTH_TOKEN}",
    "Content-Type": "application/json"
}

# === POST to register peer ===
response = requests.post(WGREST_API, headers=headers, data=json.dumps(payload))
if response.status_code != 201:
    print("❌ Failed to register peer:")
    print(response.status_code, response.text)
    exit(1)

resp_data = response.json()
safe_key = resp_data["url_safe_public_key"]

# === GET the generated .conf file ===
conf_url = WGREST_API + safe_key + "/quick.conf"
conf_response = requests.get(conf_url, headers=headers)

if conf_response.status_code == 200 and "[Interface]" in conf_response.text:
    config_text = conf_response.text

    # Return as JSON object
    output = {
        "public_key": public_key_b64,
        "conf": config_text
    }

    print("\n✅ Output JSON:")
    print(json.dumps(output, indent=2))

else:
    print("❌ Failed to fetch config file:")
    print(conf_response.status_code, conf_response.text)
