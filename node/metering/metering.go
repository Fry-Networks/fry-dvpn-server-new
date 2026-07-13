package metering

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Fry-Foundation/fry-dvpn-server-new/node/registry"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/wg"
)

type Session struct {
	PeerPublicKey string
	LastRxBytes   uint64
	LastTxBytes   uint64
	CreatedAt     time.Time
}

type Meter struct {
	mu               sync.Mutex
	wgController     wg.Controller
	registryClient   *registry.Client
	sessions         map[string]Session
	heartbeatTicker  *time.Ticker
	heartbeatInterval time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
}

func New(wgController wg.Controller, registryClient *registry.Client, heartbeatInterval time.Duration) *Meter {
	ctx, cancel := context.WithCancel(context.Background())
	return &Meter{
		wgController:      wgController,
		registryClient:    registryClient,
		sessions:          make(map[string]Session),
		heartbeatInterval:  heartbeatInterval,
		ctx:                ctx,
		cancel:             cancel,
	}
}

func (m *Meter) TrackSession(peerPublicKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[peerPublicKey]; exists {
		return fmt.Errorf("session already tracked: %s", peerPublicKey)
	}

	m.sessions[peerPublicKey] = Session{
		PeerPublicKey: peerPublicKey,
		LastRxBytes:   0,
		LastTxBytes:   0,
		CreatedAt:     time.Now(),
	}

	return nil
}

func (m *Meter) UntrackSession(peerPublicKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[peerPublicKey]; !exists {
		return fmt.Errorf("session not tracked: %s", peerPublicKey)
	}

	delete(m.sessions, peerPublicKey)
	return nil
}

func (m *Meter) GetBandwidthDelta() (uint64, error) {
	stats, err := m.wgController.Stats()
	if err != nil {
		return 0, fmt.Errorf("failed to get WG stats: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var totalDelta uint64
	for peerKey, session := range m.sessions {
		if stat, exists := stats[peerKey]; exists {
			rxDelta := stat.ReceiveBytes - session.LastRxBytes
			txDelta := stat.TransmitBytes - session.LastTxBytes

			session.LastRxBytes = stat.ReceiveBytes
			session.LastTxBytes = stat.TransmitBytes
			m.sessions[peerKey] = session

			totalDelta += rxDelta + txDelta
		}
	}

	return totalDelta, nil
}

func (m *Meter) ActiveSessionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *Meter) StartHeartbeatLoop() {
	m.heartbeatTicker = time.NewTicker(m.heartbeatInterval)
	go m.heartbeatLoop()
}

func (m *Meter) heartbeatLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.heartbeatTicker.C:
			bytesServed, err := m.GetBandwidthDelta()
			if err != nil {
				// Log error but continue
				fmt.Printf("failed to get bandwidth delta: %v\n", err)
				continue
			}

			activeSessions := uint32(m.ActiveSessionCount())

			// Submit the Proof-of-Connectivity heartbeat on-chain.
			txid, err := m.registryClient.Heartbeat(bytesServed, activeSessions)
			if err != nil {
				fmt.Printf("failed to submit heartbeat: %v\n", err)
				continue
			}
			fmt.Printf("heartbeat submitted: bytes=%d sessions=%d txid=%s\n", bytesServed, activeSessions, txid)
		}
	}
}

func (m *Meter) Stop() {
	m.cancel()
	if m.heartbeatTicker != nil {
		m.heartbeatTicker.Stop()
	}
}
