package wg

type PeerStats struct {
	PublicKey     string
	ReceiveBytes  uint64
	TransmitBytes uint64
	Endpoint      string
	AllowedIPs    []string
}

type Controller interface {
	// AddPeer adds a new WireGuard peer with the given public key and allowed IPs
	AddPeer(publicKey string, allowedIPs []string) error

	// RemovePeer removes a WireGuard peer by public key
	RemovePeer(publicKey string) error

	// Stats returns statistics for all peers
	Stats() (map[string]PeerStats, error)

	// Close closes the controller (cleanup)
	Close() error

	// InterfaceAddress returns the IP address of the WireGuard interface
	InterfaceAddress() string

	// PublicKey returns the public key of the WireGuard interface
	PublicKey() string
}
