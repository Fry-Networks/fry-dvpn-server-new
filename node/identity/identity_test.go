package identity

import (
	"encoding/base64"
	"testing"

	"github.com/algorand/go-algorand-sdk/v2/types"
	"golang.org/x/crypto/curve25519"
)

func TestGenerateProducesValidAccountAndKeys(t *testing.T) {
	dir := t.TempDir()
	id, err := New(dir, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// address must be a valid Algorand address
	if _, err := types.DecodeAddress(id.AccountAddress); err != nil {
		t.Fatalf("invalid algorand address %q: %v", id.AccountAddress, err)
	}
	// WG public key must decode to 32 bytes
	pub, err := id.WGPublicKeyBytes()
	if err != nil || len(pub) != 32 {
		t.Fatalf("wg public key not 32 bytes: %v len=%d", err, len(pub))
	}
	// account public key == decoded address public key
	apk, err := id.AccountPublicKey()
	if err != nil {
		t.Fatalf("AccountPublicKey: %v", err)
	}
	addr, _ := types.DecodeAddress(id.AccountAddress)
	if base64.StdEncoding.EncodeToString(apk) != base64.StdEncoding.EncodeToString(addr[:]) {
		t.Fatal("account public key does not match address bytes")
	}
}

func TestReloadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	id1, err := New(dir, "", "")
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	id2, err := New(dir, "", "") // reload from persisted files
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	if id1.AccountAddress != id2.AccountAddress {
		t.Fatalf("address changed on reload: %s != %s", id1.AccountAddress, id2.AccountAddress)
	}
	if id1.WGPublicKey != id2.WGPublicKey || id1.WGPrivateKey != id2.WGPrivateKey {
		t.Fatal("wg keys changed on reload")
	}
}

func TestWGPublicKeyMatchesCurve25519(t *testing.T) {
	dir := t.TempDir()
	id, err := New(dir, "", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	priv, _ := base64.StdEncoding.DecodeString(id.WGPrivateKey)
	want, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatalf("curve25519: %v", err)
	}
	if base64.StdEncoding.EncodeToString(want) != id.WGPublicKey {
		t.Fatal("stored WG public key does not match curve25519(private, basepoint)")
	}
}

func TestProvidedWGKeyIsUsed(t *testing.T) {
	dir := t.TempDir()
	priv := make([]byte, 32)
	for i := range priv {
		priv[i] = byte(i + 1)
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	privB64 := base64.StdEncoding.EncodeToString(priv)
	id, err := New(dir, "", privB64)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if id.WGPrivateKey != privB64 {
		t.Fatal("provided WG private key not used")
	}
}
