package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Fry-Foundation/fry-dvpn-server-new/node/api"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/config"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/identity"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/ippool"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/metering"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/registry"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/wg"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Create identity (account + WG keypair)
	id, err := identity.New("node-identity", cfg.NodeMnemonic, cfg.WGPrivateKey)
	if err != nil {
		log.Fatalf("failed to create identity: %v", err)
	}

	log.Printf("Node address: %s", id.AccountAddress)
	log.Printf("WG public key: %s", id.WGPublicKey)

	// Create registry client
	registryClient, err := registry.New(cfg.AlgodURL(), cfg.AlgodToken, cfg.RegistryAppID, cfg.FVPNAsaID, id.Account)
	if err != nil {
		log.Fatalf("failed to create registry client: %v", err)
	}
	defer registryClient.Close()

	// Create WireGuard controller (mock for now)
	wgController := wg.NewMock(
		fmt.Sprintf("10.7.0.1/24"),
		id.WGPublicKey,
	)
	defer wgController.Close()

	// Create IP pool
	pool, err := ippool.New("10.7.0.0/24", true)
	if err != nil {
		log.Fatalf("failed to create IP pool: %v", err)
	}

	// Create API server
	mux := http.NewServeMux()
	apiServer := api.New(
		wgController,
		registryClient,
		pool,
		id.AccountAddress,
		id.WGPublicKey,
		cfg.PublicEndpoint,
		cfg.FVPNAsaID,
		cfg.PricePerGB,
	)
	apiServer.RegisterHandlers(mux)

	// Create metering service
	meter := metering.New(
		wgController,
		registryClient,
		time.Duration(cfg.HeartbeatSeconds)*time.Second,
	)
	meter.StartHeartbeatLoop()
	defer meter.Stop()

	// HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.APIPort),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server in a goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("starting API server on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	// Register node on-chain
	if err := registerNodeOnChain(registryClient, cfg, id); err != nil {
		log.Printf("warning: failed to register node on-chain: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("received shutdown signal")
	case err := <-serverErrors:
		log.Fatalf("server error: %v", err)
	}

	// Graceful shutdown
	log.Println("shutting down...")

	// Deregister node
	if err := deregisterNodeOnChain(registryClient); err != nil {
		log.Printf("warning: failed to deregister node: %v", err)
	}

	// Stop HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("error during shutdown: %v", err)
	}

	log.Println("shutdown complete")
}

func registerNodeOnChain(rc *registry.Client, cfg *config.Config, id *identity.Identity) error {
	txid, err := rc.RegisterNode(
		id.WGPublicKey,
		cfg.PublicEndpoint,
		cfg.Region,
		cfg.CapacityMbps,
		cfg.PricePerGB,
	)
	if err != nil {
		return fmt.Errorf("register node on-chain: %w", err)
	}
	log.Printf("node registered on-chain (txid=%s)", txid)
	return nil
}

func deregisterNodeOnChain(rc *registry.Client) error {
	txid, err := rc.Deregister()
	if err != nil {
		return fmt.Errorf("deregister node on-chain: %w", err)
	}
	log.Printf("node deregistered on-chain (txid=%s)", txid)
	return nil
}
