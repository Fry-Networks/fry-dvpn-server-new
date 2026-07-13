// Package identity manages the node's cryptographic identity: a real Algorand
// account (used to sign registry transactions and to be the box key / payment
// recipient) and a real WireGuard Curve25519 keypair (advertised to clients so
// the tunnel can be established). Secrets are persisted 0600 and never logged.
package identity

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/mnemonic"
	"golang.org/x/crypto/curve25519"
)

const (
	accountFile = "account.txt" // 25-word Algorand mnemonic
	wgKeyFile   = "wg.key"      // base64 Curve25519 private key
	wgPubFile   = "wg.pub"      // base64 Curve25519 public key
)

// Identity holds the node's signing account and WireGuard keys.
type Identity struct {
	Account        crypto.Account // real Algorand account (private key held for signing)
	AccountAddress string         // account.Address.String()
	WGPrivateKey   string         // base64 Curve25519 private key
	WGPublicKey    string         // base64 Curve25519 public key
	IdentityDir    string
}

// New loads the identity from identityDir, or generates and persists a fresh one.
// providedMnemonic / providedWGPrivateKey (from env) take precedence when set.
func New(identityDir, providedMnemonic, providedWGPrivateKey string) (*Identity, error) {
	if err := os.MkdirAll(identityDir, 0o700); err != nil {
		return nil, fmt.Errorf("create identity dir: %w", err)
	}
	id := &Identity{IdentityDir: identityDir}

	// ---- Algorand account ----
	mn := strings.TrimSpace(providedMnemonic)
	if mn == "" {
		acctPath := filepath.Join(identityDir, accountFile)
		if data, err := os.ReadFile(acctPath); err == nil {
			mn = strings.TrimSpace(string(data))
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read account file: %w", err)
		} else {
			acct := crypto.GenerateAccount()
			generated, err := mnemonic.FromPrivateKey(acct.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("derive mnemonic: %w", err)
			}
			mn = generated
			if err := os.WriteFile(acctPath, []byte(mn+"\n"), 0o600); err != nil {
				return nil, fmt.Errorf("persist account: %w", err)
			}
		}
	}
	sk, err := mnemonic.ToPrivateKey(mn)
	if err != nil {
		return nil, fmt.Errorf("invalid account mnemonic: %w", err)
	}
	acct, err := crypto.AccountFromPrivateKey(sk)
	if err != nil {
		return nil, fmt.Errorf("account from private key: %w", err)
	}
	id.Account = acct
	id.AccountAddress = acct.Address.String()

	// ---- WireGuard Curve25519 keypair ----
	wgPriv := strings.TrimSpace(providedWGPrivateKey)
	if wgPriv == "" {
		wgKeyPath := filepath.Join(identityDir, wgKeyFile)
		if data, err := os.ReadFile(wgKeyPath); err == nil {
			wgPriv = strings.TrimSpace(string(data))
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read wg key: %w", err)
		} else {
			generated, err := generateWGPrivateKey()
			if err != nil {
				return nil, fmt.Errorf("generate wg key: %w", err)
			}
			wgPriv = generated
			if err := os.WriteFile(wgKeyPath, []byte(wgPriv+"\n"), 0o600); err != nil {
				return nil, fmt.Errorf("persist wg key: %w", err)
			}
		}
	}
	id.WGPrivateKey = wgPriv
	pub, err := wgPublicKey(wgPriv)
	if err != nil {
		return nil, fmt.Errorf("derive wg public key: %w", err)
	}
	id.WGPublicKey = pub
	// persist public key for convenience (non-secret)
	_ = os.WriteFile(filepath.Join(identityDir, wgPubFile), []byte(pub+"\n"), 0o644)

	return id, nil
}

// AccountPublicKey returns the 32-byte Ed25519 public key of the account, which is
// exactly the NodeRegistry box key for this node.
func (id *Identity) AccountPublicKey() ([]byte, error) {
	if len(id.Account.PublicKey) != 32 {
		return nil, errors.New("account public key not initialized")
	}
	out := make([]byte, 32)
	copy(out, id.Account.PublicKey)
	return out, nil
}

// WGPublicKeyBytes returns the raw 32-byte Curve25519 public key.
func (id *Identity) WGPublicKeyBytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(id.WGPublicKey)
}

func generateWGPrivateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	// WireGuard clamps the private scalar.
	key[0] &= 248
	key[31] &= 127
	key[31] |= 64
	return base64.StdEncoding.EncodeToString(key), nil
}

func wgPublicKey(privB64 string) (string, error) {
	priv, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil {
		return "", fmt.Errorf("decode wg private key: %w", err)
	}
	if len(priv) != 32 {
		return "", fmt.Errorf("wg private key must be 32 bytes, got %d", len(priv))
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return "", fmt.Errorf("curve25519: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}
