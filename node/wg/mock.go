package wg

import (
	"fmt"
	"sync"
)

type MockController struct {
	mu                sync.Mutex
	peers             map[string]PeerStats
	interfaceAddress  string
	interfacePublicKey string
}

func NewMock(interfaceAddress, publicKey string) *MockController {
	return &MockController{
		peers:              make(map[string]PeerStats),
		interfaceAddress:   interfaceAddress,
		interfacePublicKey: publicKey,
	}
}

func (m *MockController) AddPeer(publicKey string, allowedIPs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.peers[publicKey]; exists {
		return fmt.Errorf("peer already exists: %s", publicKey)
	}

	m.peers[publicKey] = PeerStats{
		PublicKey:    publicKey,
		AllowedIPs:   allowedIPs,
		ReceiveBytes: 0,
		TransmitBytes: 0,
	}

	return nil
}

func (m *MockController) RemovePeer(publicKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.peers[publicKey]; !exists {
		return fmt.Errorf("peer not found: %s", publicKey)
	}

	delete(m.peers, publicKey)
	return nil
}

func (m *MockController) Stats() (map[string]PeerStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]PeerStats)
	for k, v := range m.peers {
		result[k] = v
	}

	return result, nil
}

func (m *MockController) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peers = make(map[string]PeerStats)
	return nil
}

func (m *MockController) InterfaceAddress() string {
	return m.interfaceAddress
}

func (m *MockController) PublicKey() string {
	return m.interfacePublicKey
}

// Testing helper: increment peer stats
func (m *MockController) IncrementStats(publicKey string, rxBytes, txBytes uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	peer, exists := m.peers[publicKey]
	if !exists {
		return fmt.Errorf("peer not found: %s", publicKey)
	}

	peer.ReceiveBytes += rxBytes
	peer.TransmitBytes += txBytes
	m.peers[publicKey] = peer

	return nil
}
