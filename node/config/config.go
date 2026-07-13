package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Algorand
	AlgodServer   string
	AlgodPort     string
	AlgodToken    string
	RegistryAppID uint64
	FVPNAsaID     uint64

	// Identity
	NodeMnemonic  string
	WGPrivateKey  string

	// Network
	PublicEndpoint    string
	Region            string
	CapacityMbps      uint32
	PricePerGB        uint64
	WGPort            uint16
	APIPort           uint16
	HeartbeatSeconds  uint32
	AllowFullTunnel   bool

	// WireGuard
	WGInterfaceName string
}

func Load() (*Config, error) {
	c := &Config{
		// Defaults
		AlgodServer:     "http://localhost",
		AlgodPort:       "4001",
		AlgodToken:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FVPNAsaID:       2485198745,
		WGPort:          51820,
		APIPort:         8088,
		HeartbeatSeconds: 300,
		WGInterfaceName: "wg0",
		AllowFullTunnel: false,
	}

	// Parse flags
	flag.StringVar(&c.AlgodServer, "algod-server", c.AlgodServer, "Algorand node server")
	flag.StringVar(&c.AlgodPort, "algod-port", c.AlgodPort, "Algorand node port")
	flag.StringVar(&c.AlgodToken, "algod-token", c.AlgodToken, "Algorand node token")

	var registryAppID, fvpnAsaID, pricePerGB uint64
	var capacityMbps, heartbeatSeconds uint
	var wgPort, apiPort uint

	flag.Uint64Var(&registryAppID, "registry-app-id", c.RegistryAppID, "NodeRegistry app ID")
	flag.Uint64Var(&fvpnAsaID, "fvpn-asa-id", c.FVPNAsaID, "fVPN asset ID")
	flag.StringVar(&c.NodeMnemonic, "node-mnemonic", "", "Node account mnemonic (optional; generates if not provided)")
	flag.StringVar(&c.WGPrivateKey, "wg-private-key", "", "WireGuard private key (optional; generates if not provided)")
	flag.StringVar(&c.PublicEndpoint, "public-endpoint", "", "Public endpoint (host:port)")
	flag.StringVar(&c.Region, "region", c.Region, "Node region")
	flag.UintVar(&capacityMbps, "capacity-mbps", uint(c.CapacityMbps), "Capacity in Mbps")
	flag.Uint64Var(&pricePerGB, "price-per-gb", c.PricePerGB, "Price in fVPN microunits per GB")
	flag.UintVar(&wgPort, "wg-port", uint(c.WGPort), "WireGuard port")
	flag.UintVar(&apiPort, "api-port", uint(c.APIPort), "API server port")
	flag.UintVar(&heartbeatSeconds, "heartbeat-seconds", uint(c.HeartbeatSeconds), "Heartbeat interval in seconds")
	flag.BoolVar(&c.AllowFullTunnel, "allow-full-tunnel", c.AllowFullTunnel, "Allow full tunnel egress")
	flag.Parse()

	if registryAppID > 0 {
		c.RegistryAppID = registryAppID
	}
	if fvpnAsaID > 0 {
		c.FVPNAsaID = fvpnAsaID
	}
	if capacityMbps > 0 {
		c.CapacityMbps = uint32(capacityMbps)
	}
	c.PricePerGB = pricePerGB
	c.WGPort = uint16(wgPort)
	c.APIPort = uint16(apiPort)
	c.HeartbeatSeconds = uint32(heartbeatSeconds)

	// Override from environment
	if v := os.Getenv("ALGOD_SERVER"); v != "" {
		c.AlgodServer = v
	}
	if v := os.Getenv("ALGOD_PORT"); v != "" {
		c.AlgodPort = v
	}
	if v := os.Getenv("ALGOD_TOKEN"); v != "" {
		c.AlgodToken = v
	}
	if v := os.Getenv("REGISTRY_APP_ID"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid REGISTRY_APP_ID: %w", err)
		}
		c.RegistryAppID = id
	}
	if v := os.Getenv("FVPN_ASA_ID"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid FVPN_ASA_ID: %w", err)
		}
		c.FVPNAsaID = id
	}
	if v := os.Getenv("NODE_MNEMONIC"); v != "" {
		c.NodeMnemonic = v
	}
	if v := os.Getenv("WG_PRIVATE_KEY"); v != "" {
		c.WGPrivateKey = v
	}
	if v := os.Getenv("PUBLIC_ENDPOINT"); v != "" {
		c.PublicEndpoint = v
	}
	if v := os.Getenv("REGION"); v != "" {
		c.Region = v
	}
	if v := os.Getenv("CAPACITY_MBPS"); v != "" {
		cap, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid CAPACITY_MBPS: %w", err)
		}
		c.CapacityMbps = uint32(cap)
	}
	if v := os.Getenv("PRICE_PER_GB"); v != "" {
		price, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid PRICE_PER_GB: %w", err)
		}
		c.PricePerGB = price
	}
	if v := os.Getenv("WG_PORT"); v != "" {
		port, err := strconv.ParseUint(v, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid WG_PORT: %w", err)
		}
		c.WGPort = uint16(port)
	}
	if v := os.Getenv("API_PORT"); v != "" {
		port, err := strconv.ParseUint(v, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid API_PORT: %w", err)
		}
		c.APIPort = uint16(port)
	}
	if v := os.Getenv("HEARTBEAT_SECONDS"); v != "" {
		hb, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid HEARTBEAT_SECONDS: %w", err)
		}
		c.HeartbeatSeconds = uint32(hb)
	}
	if v := os.Getenv("ALLOW_FULL_TUNNEL"); v != "" {
		c.AllowFullTunnel = strings.ToLower(v) == "true" || v == "1"
	}

	// Validate
	if err := c.Validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Config) Validate() error {
	if c.RegistryAppID == 0 {
		return errors.New("REGISTRY_APP_ID is required")
	}
	if c.Region == "" {
		return errors.New("REGION is required")
	}
	if c.CapacityMbps == 0 {
		return errors.New("CAPACITY_MBPS is required (must be > 0)")
	}
	if c.AlgodServer == "" || c.AlgodPort == "" {
		return errors.New("ALGOD_SERVER and ALGOD_PORT are required")
	}
	return nil
}

func (c *Config) AlgodURL() string {
	return c.AlgodServer + ":" + c.AlgodPort
}
