package wg

import (
	"testing"
)

func TestMockAddPeer(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	err := mock.AddPeer("peer1", []string{"10.7.0.2/32"})
	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	// Try to add same peer again
	err = mock.AddPeer("peer1", []string{"10.7.0.3/32"})
	if err == nil {
		t.Errorf("expected error when adding duplicate peer")
	}
}

func TestMockRemovePeer(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	err := mock.AddPeer("peer1", []string{"10.7.0.2/32"})
	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	err = mock.RemovePeer("peer1")
	if err != nil {
		t.Fatalf("failed to remove peer: %v", err)
	}

	// Try to remove non-existent peer
	err = mock.RemovePeer("nonexistent")
	if err == nil {
		t.Errorf("expected error when removing non-existent peer")
	}
}

func TestMockStats(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	err := mock.AddPeer("peer1", []string{"10.7.0.2/32"})
	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	stats, err := mock.Stats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if len(stats) != 1 {
		t.Errorf("expected 1 peer in stats, got %d", len(stats))
	}

	if _, exists := stats["peer1"]; !exists {
		t.Errorf("peer1 not found in stats")
	}
}

func TestMockIncrementStats(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	err := mock.AddPeer("peer1", []string{"10.7.0.2/32"})
	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	err = mock.IncrementStats("peer1", 1000, 2000)
	if err != nil {
		t.Fatalf("failed to increment stats: %v", err)
	}

	stats, _ := mock.Stats()
	if stats["peer1"].ReceiveBytes != 1000 {
		t.Errorf("expected 1000 RX bytes, got %d", stats["peer1"].ReceiveBytes)
	}

	if stats["peer1"].TransmitBytes != 2000 {
		t.Errorf("expected 2000 TX bytes, got %d", stats["peer1"].TransmitBytes)
	}

	// Try to increment non-existent peer
	err = mock.IncrementStats("nonexistent", 100, 200)
	if err == nil {
		t.Errorf("expected error when incrementing non-existent peer")
	}
}

func TestMockInterfaceAddress(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	addr := mock.InterfaceAddress()
	if addr != "10.7.0.1/24" {
		t.Errorf("expected 10.7.0.1/24, got %s", addr)
	}
}

func TestMockPublicKey(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	pubKey := mock.PublicKey()
	if pubKey != "testPubKey123" {
		t.Errorf("expected testPubKey123, got %s", pubKey)
	}
}

func TestMockClose(t *testing.T) {
	mock := NewMock("10.7.0.1/24", "testPubKey123")

	mock.AddPeer("peer1", []string{"10.7.0.2/32"})

	err := mock.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	// After close, peers should be cleared
	stats, _ := mock.Stats()
	if len(stats) != 0 {
		t.Errorf("expected 0 peers after close, got %d", len(stats))
	}
}
