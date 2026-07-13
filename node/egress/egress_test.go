package egress

import (
	"strings"
	"testing"
)

func TestPolicyNewDefault(t *testing.T) {
	p := NewPolicy(false)

	if p.Mode != DenyRFC1918 {
		t.Errorf("expected DenyRFC1918 mode, got %s", p.Mode)
	}

	if p.AllowFullTunnel {
		t.Errorf("expected AllowFullTunnel to be false")
	}

	if len(p.BlockedRanges) == 0 {
		t.Errorf("expected blocked ranges to be set")
	}

	if len(p.BlockedPorts) == 0 {
		t.Errorf("expected blocked ports to be set")
	}
}

func TestPolicyFullTunnel(t *testing.T) {
	p := NewPolicy(true)

	if p.Mode != AllowAll {
		t.Errorf("expected AllowAll mode, got %s", p.Mode)
	}

	if !p.AllowFullTunnel {
		t.Errorf("expected AllowFullTunnel to be true")
	}
}

func TestGenerateRulesAllowAll(t *testing.T) {
	p := NewPolicy(true)

	rules := p.GenerateRules()

	if !strings.Contains(rules, "nft add rule inet filter forward oifname wg0 accept") {
		t.Errorf("expected accept rule in AllowAll mode, got:\n%s", rules)
	}

	if strings.Contains(rules, "drop") {
		t.Errorf("unexpected drop rule in AllowAll mode:\n%s", rules)
	}
}

func TestGenerateRulesDenyRFC1918(t *testing.T) {
	p := NewPolicy(false)

	rules := p.GenerateRules()

	// Should have drop rules for RFC1918
	if !strings.Contains(rules, "10.0.0.0/8") {
		t.Errorf("missing rule for 10.0.0.0/8 range:\n%s", rules)
	}

	if !strings.Contains(rules, "172.16.0.0/12") {
		t.Errorf("missing rule for 172.16.0.0/12 range:\n%s", rules)
	}

	if !strings.Contains(rules, "192.168.0.0/16") {
		t.Errorf("missing rule for 192.168.0.0/16 range:\n%s", rules)
	}

	// Should have drop for common abuse ports
	if !strings.Contains(rules, "tcp dport 25") {
		t.Errorf("missing SMTP port block:\n%s", rules)
	}

	if !strings.Contains(rules, "tcp dport 23") {
		t.Errorf("missing Telnet port block:\n%s", rules)
	}
}

func TestValidateRules(t *testing.T) {
	p := NewPolicy(false)

	err := p.ValidateRules()
	if err != nil {
		t.Errorf("valid policy failed validation: %v", err)
	}

	// Test invalid mode
	p.Mode = "invalid"
	err = p.ValidateRules()
	if err == nil {
		t.Errorf("expected validation error for invalid mode")
	}

	// Test empty mode
	p.Mode = ""
	err = p.ValidateRules()
	if err == nil {
		t.Errorf("expected validation error for empty mode")
	}
}

func TestBlockedPortsInRules(t *testing.T) {
	p := NewPolicy(false)
	p.BlockedPorts = []uint16{80, 443, 8080}

	rules := p.GenerateRules()

	if !strings.Contains(rules, "tcp dport 80") {
		t.Errorf("missing block for port 80")
	}

	if !strings.Contains(rules, "tcp dport 443") {
		t.Errorf("missing block for port 443")
	}

	if !strings.Contains(rules, "tcp dport 8080") {
		t.Errorf("missing block for port 8080")
	}
}

func TestCustomBlockedRanges(t *testing.T) {
	p := NewPolicy(false)
	p.BlockedRanges = []string{"192.0.2.0/24", "198.51.100.0/24"}

	rules := p.GenerateRules()

	if !strings.Contains(rules, "192.0.2.0/24") {
		t.Errorf("missing custom range 192.0.2.0/24")
	}

	if !strings.Contains(rules, "198.51.100.0/24") {
		t.Errorf("missing custom range 198.51.100.0/24")
	}
}
