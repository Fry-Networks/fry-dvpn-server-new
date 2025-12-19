import nacl.public, base64, os

# Generate private key
private_key = nacl.public.PrivateKey.generate()
private_key_b64 = base64.b64encode(bytes(private_key)).decode()

# Get public key
public_key = base64.b64encode(bytes(private_key.public_key)).decode()

# Generate preshared key
preshared_key = base64.b64encode(os.urandom(32)).decode()

print("Private Key   :", private_key_b64)
print("Public Key    :", public_key)
print("Preshared Key :", preshared_key)
