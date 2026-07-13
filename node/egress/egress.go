package egress

import (
	"fmt"
	"strings"
)

const (
	DenyRFC1918 = "deny_rfc1918"
	DenyAbuse   = "deny_abuse"
	AllowAll    = "allow_all"
)

type Policy struct {
	Mode              string   // DenyRFC1918 | DenyAbuse | AllowAll
	BlockedPorts      []uint16 // e.g., [23, 25, 53]
	BlockedRanges     []string // e.g., ["10.0.0.0/8", "172.16.0.0/12"]
	AllowedRanges     []string // e.g., ["8.8.8.0/24"]
	AllowFullTunnel   bool
}

func NewPolicy(allowFullTunnel bool) *Policy {
	p := &Policy{
		Mode:            DenyRFC1918,
		AllowFullTunnel: allowFullTunnel,
	}

	if allowFullTunnel {
		p.Mode = AllowAll
	}

	// Default blocked ports: Telnet, SMTP, DNS (when used for abuse), DHCP
	p.BlockedPorts = []uint16{23, 25, 53, 67, 68}

	// Default RFC1918 ranges (private networks)
	p.BlockedRanges = []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",    // Loopback
		"169.254.0.0/16", // Link-local
		"224.0.0.0/4",    // Multicast
		"240.0.0.0/4",    // Reserved
	}

	return p
}

func (p *Policy) GenerateRules() string {
	var rules strings.Builder

	rules.WriteString("# frynode egress policy\n")
	rules.WriteString(fmt.Sprintf("# Mode: %s\n", p.Mode))
	rules.WriteString(fmt.Sprintf("# Full tunnel allowed: %v\n", p.AllowFullTunnel))
	rules.WriteString("\n")

	switch p.Mode {
	case AllowAll:
		rules.WriteString("# Full tunnel mode: all egress allowed\n")
		rules.WriteString("nft add rule inet filter forward oifname wg0 accept\n")

	case DenyRFC1918:
		rules.WriteString("# Split tunnel: deny RFC1918 and blocked ports\n")
		rules.WriteString("# Block RFC1918 ranges\n")
		for _, r := range p.BlockedRanges {
			rules.WriteString(fmt.Sprintf("nft add rule inet filter forward oifname wg0 ip daddr %s drop\n", r))
		}

		rules.WriteString("# Block specific ports\n")
		for _, port := range p.BlockedPorts {
			rules.WriteString(fmt.Sprintf("nft add rule inet filter forward oifname wg0 tcp dport %d drop\n", port))
			rules.WriteString(fmt.Sprintf("nft add rule inet filter forward oifname wg0 udp dport %d drop\n", port))
		}

		rules.WriteString("# Allow everything else\n")
		rules.WriteString("nft add rule inet filter forward oifname wg0 accept\n")

	case DenyAbuse:
		rules.WriteString("# Abuse mitigation: aggressive blocking\n")
		for _, r := range p.BlockedRanges {
			rules.WriteString(fmt.Sprintf("nft add rule inet filter forward oifname wg0 ip daddr %s drop\n", r))
		}
		for _, port := range p.BlockedPorts {
			rules.WriteString(fmt.Sprintf("nft add rule inet filter forward oifname wg0 tcp dport %d drop\n", port))
			rules.WriteString(fmt.Sprintf("nft add rule inet filter forward oifname wg0 udp dport %d drop\n", port))
		}
		rules.WriteString("nft add rule inet filter forward oifname wg0 accept\n")
	}

	return rules.String()
}

func (p *Policy) ValidateRules() error {
	if p.Mode == "" {
		return fmt.Errorf("mode is required")
	}

	validModes := map[string]bool{
		AllowAll:    true,
		DenyRFC1918: true,
		DenyAbuse:   true,
	}

	if !validModes[p.Mode] {
		return fmt.Errorf("invalid mode: %s", p.Mode)
	}

	if len(p.BlockedRanges) == 0 && p.Mode != AllowAll {
		return fmt.Errorf("blocked ranges required for mode %s", p.Mode)
	}

	return nil
}
