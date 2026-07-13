package registry

import (
	"testing"

	"github.com/algorand/go-algorand-sdk/v2/abi"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
)

// The ABI signatures must parse and match the NodeRegistry contract's methods.
func TestMethodSignaturesParse(t *testing.T) {
	cases := []struct {
		sig       string
		wantArgs  int
	}{
		{sigRegister, 6},
		{sigUpdate, 4},
		{sigHeartbeat, 2},
		{sigSetStatus, 1},
		{sigDeregister, 0},
	}
	for _, c := range cases {
		m, err := abi.MethodFromSignature(c.sig)
		if err != nil {
			t.Fatalf("MethodFromSignature(%q): %v", c.sig, err)
		}
		if len(m.Args) != c.wantArgs {
			t.Fatalf("%q: got %d args, want %d", c.sig, len(m.Args), c.wantArgs)
		}
	}
}

func TestNewDerivesAppAddress(t *testing.T) {
	acct := crypto.GenerateAccount()
	c, err := New("http://localhost:4001", "", 1234, 2485198745, acct)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.appAddr != crypto.GetApplicationAddress(1234) {
		t.Fatal("app address mismatch")
	}
	if c.fvpnASA != 2485198745 {
		t.Fatal("fvpn asa not set")
	}
}

func TestVerifyPaymentRejectsEmptyTxid(t *testing.T) {
	acct := crypto.GenerateAccount()
	c, err := New("http://localhost:4001", "", 1234, 2485198745, acct)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ok, err := c.VerifyPayment("", 2485198745, 1); ok || err == nil {
		t.Fatal("expected error for empty txid")
	}
}

func TestRegisterRejectsBadWGKey(t *testing.T) {
	acct := crypto.GenerateAccount()
	c, _ := New("http://localhost:4001", "", 1234, 2485198745, acct)
	if _, err := c.RegisterNode("not-base64!!", "1.2.3.4:51820", "us-east", 100, 1_000_000); err == nil {
		t.Fatal("expected error for invalid wg pubkey")
	}
}
