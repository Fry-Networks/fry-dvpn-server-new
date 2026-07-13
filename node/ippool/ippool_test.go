package ippool

import (
	"strings"
	"testing"
)

func TestIPPoolAllocate(t *testing.T) {
	pool, err := New("10.7.0.0/24", true)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// Allocate first IP (should be .1 after gateway)
	ip1, err := pool.Allocate()
	if err != nil {
		t.Fatalf("failed to allocate first IP: %v", err)
	}

	if !strings.Contains(ip1, "10.7.0.1") {
		t.Errorf("expected 10.7.0.1, got %s", ip1)
	}

	// Allocate second IP
	ip2, err := pool.Allocate()
	if err != nil {
		t.Fatalf("failed to allocate second IP: %v", err)
	}

	if !strings.Contains(ip2, "10.7.0.2") {
		t.Errorf("expected 10.7.0.2, got %s", ip2)
	}

	// Ensure different
	if ip1 == ip2 {
		t.Errorf("allocated same IP twice: %s", ip1)
	}
}

func TestIPPoolFree(t *testing.T) {
	pool, err := New("10.7.0.0/24", true)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	ip, err := pool.Allocate()
	if err != nil {
		t.Fatalf("failed to allocate IP: %v", err)
	}

	allocated := pool.Allocated()
	if allocated != 3 { // gateway + broadcast + 1 allocated
		t.Errorf("expected 3 allocated, got %d", allocated)
	}

	err = pool.Free(ip)
	if err != nil {
		t.Fatalf("failed to free IP: %v", err)
	}

	allocated = pool.Allocated()
	if allocated != 2 { // just gateway + broadcast
		t.Errorf("expected 2 allocated after free, got %d", allocated)
	}
}

func TestIPPoolExhaustion(t *testing.T) {
	pool, err := New("10.7.0.0/24", true)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	// Allocate until exhausted (256 - 2 for gateway/broadcast = 254 usable)
	for i := 0; i < 254; i++ {
		_, err := pool.Allocate()
		if err != nil {
			t.Fatalf("failed to allocate IP %d: %v", i, err)
		}
	}

	// Next allocation should fail
	_, err = pool.Allocate()
	if err == nil {
		t.Errorf("expected exhaustion error, got nil")
	}
}

func TestIPPoolAvailable(t *testing.T) {
	pool, err := New("10.7.0.0/24", true)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	available := pool.Available()
	if available != 254 {
		t.Errorf("expected 254 available (256-2), got %d", available)
	}

	ip, _ := pool.Allocate()
	available = pool.Available()
	if available != 253 {
		t.Errorf("expected 253 after allocation, got %d", available)
	}

	pool.Free(ip)
	available = pool.Available()
	if available != 254 {
		t.Errorf("expected 254 after free, got %d", available)
	}
}
