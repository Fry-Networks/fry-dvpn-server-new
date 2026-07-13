package metering

import (
	"testing"
	"time"

	"github.com/Fry-Foundation/fry-dvpn-server-new/node/wg"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
)

func TestMeterTrackSession(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "pubkey")
	meter := New(mock, nil, 60*time.Second)

	peerKey := "peer1"
	err := meter.TrackSession(peerKey)
	if err != nil {
		t.Fatalf("failed to track session: %v", err)
	}

	// Try to track again
	err = meter.TrackSession(peerKey)
	if err == nil {
		t.Errorf("expected error tracking duplicate session")
	}

	if meter.ActiveSessionCount() != 1 {
		t.Errorf("expected 1 active session, got %d", meter.ActiveSessionCount())
	}
}

func TestMeterUntrackSession(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "pubkey")
	meter := New(mock, nil, 60*time.Second)

	peerKey := "peer1"
	meter.TrackSession(peerKey)

	err := meter.UntrackSession(peerKey)
	if err != nil {
		t.Fatalf("failed to untrack session: %v", err)
	}

	if meter.ActiveSessionCount() != 0 {
		t.Errorf("expected 0 active sessions after untrack, got %d", meter.ActiveSessionCount())
	}

	// Try to untrack again
	err = meter.UntrackSession(peerKey)
	if err == nil {
		t.Errorf("expected error untracking non-existent session")
	}
}

func TestMeterGetBandwidthDelta(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "pubkey")
	meter := New(mock, nil, 60*time.Second)

	peerKey := "peer1"
	meter.TrackSession(peerKey)

	// Add peer to WG mock
	mock.AddPeer(peerKey, []string{"10.7.0.2/32"})

	// Initial bandwidth should be 0
	delta, err := meter.GetBandwidthDelta()
	if err != nil {
		t.Fatalf("failed to get bandwidth delta: %v", err)
	}

	if delta != 0 {
		t.Errorf("expected 0 initial delta, got %d", delta)
	}

	// Increment stats
	mock.IncrementStats(peerKey, 1000, 2000)

	// Now delta should reflect the change
	delta, err = meter.GetBandwidthDelta()
	if err != nil {
		t.Fatalf("failed to get bandwidth delta: %v", err)
	}

	if delta != 3000 {
		t.Errorf("expected 3000 delta, got %d", delta)
	}

	// Next call should return 0 (no new data)
	delta, err = meter.GetBandwidthDelta()
	if err != nil {
		t.Fatalf("failed to get bandwidth delta: %v", err)
	}

	if delta != 0 {
		t.Errorf("expected 0 delta on second call, got %d", delta)
	}
}

func TestMeterActiveSessionCount(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "pubkey")
	meter := New(mock, nil, 60*time.Second)

	if meter.ActiveSessionCount() != 0 {
		t.Errorf("expected 0 initial sessions")
	}

	for i := 1; i <= 5; i++ {
		peerKey := string(rune('a' + i - 1))
		meter.TrackSession(peerKey)

		if meter.ActiveSessionCount() != i {
			t.Errorf("expected %d active sessions, got %d", i, meter.ActiveSessionCount())
		}
	}
}

func TestMeterStop(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "pubkey")
	meter := New(mock, nil, 60*time.Second)

	meter.Stop()

	// After stop, context should be cancelled
	select {
	case <-meter.ctx.Done():
		// Expected
	default:
		t.Errorf("expected context to be cancelled")
	}
}

func TestMeterWithRealAccount(t *testing.T) {
	// Test with actual Algorand account creation
	account := crypto.GenerateAccount()

	if account.Address.String() == "" {
		t.Errorf("account address is empty")
	}

	mock := wg.NewMock("10.7.0.1/24", "pubkey")
	meter := New(mock, nil, 60*time.Second)

	peerKey := "peer1"
	err := meter.TrackSession(peerKey)
	if err != nil {
		t.Fatalf("failed to track session: %v", err)
	}

	if meter.ActiveSessionCount() != 1 {
		t.Errorf("expected 1 active session")
	}
}
