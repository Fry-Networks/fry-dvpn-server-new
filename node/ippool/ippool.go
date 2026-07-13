package ippool

import (
	"fmt"
	"net"
	"sync"
)

type IPPool struct {
	mu        sync.Mutex
	allocated map[string]bool // IP -> allocated
	base      net.IP
	mask      net.IPMask
	hosts     int
}

func New(cidr string, excludeGW bool) (*IPPool, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	pool := &IPPool{
		allocated: make(map[string]bool),
		base:      network.IP,
		mask:      network.Mask,
		hosts:     256,
	}

	if excludeGW {
		// Mark gateway (.0) and broadcast (.255) as used
		pool.allocated[net.IP([]byte{network.IP[0], network.IP[1], network.IP[2], network.IP[3]}).String()] = true
		pool.allocated[net.IP([]byte{network.IP[0], network.IP[1], network.IP[2], 255}).String()] = true
	}

	_ = ip // use ip to validate parsing
	return pool, nil
}

func (p *IPPool) Allocate() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Find first available IP starting at .1
	for i := 1; i < p.hosts; i++ {
		candidate := net.IP([]byte{p.base[0], p.base[1], p.base[2], byte(i)})
		candStr := candidate.String()
		if !p.allocated[candStr] {
			p.allocated[candStr] = true
			return candStr + "/32", nil
		}
	}

	return "", fmt.Errorf("IP pool exhausted")
}

func (p *IPPool) Free(ipWithMask string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Parse IP without mask
	ip, _, err := net.ParseCIDR(ipWithMask)
	if err != nil {
		// Try parsing as plain IP
		ip = net.ParseIP(ipWithMask)
		if ip == nil {
			return fmt.Errorf("invalid IP: %s", ipWithMask)
		}
	}

	ipStr := ip.String()
	if !p.allocated[ipStr] {
		return fmt.Errorf("IP not allocated: %s", ipStr)
	}

	delete(p.allocated, ipStr)
	return nil
}

func (p *IPPool) Allocated() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.allocated)
}

func (p *IPPool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hosts - len(p.allocated)
}
