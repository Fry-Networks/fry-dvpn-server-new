package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/Fry-Foundation/fry-dvpn-server-new/node/ippool"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/registry"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/wg"
)

type Challenge struct {
	Value   string
	Nonce   []byte
	Expires time.Time
}

type SessionData struct {
	ID         string
	Wallet     string
	ClientPubKey string
	IPAddress  string
	CreatedAt  time.Time
}

type Server struct {
	mu              sync.RWMutex
	wgController    wg.Controller
	registryClient  *registry.Client
	ipPool          *ippool.IPPool
	challenges      map[string]Challenge
	sessions        map[string]SessionData
	usedPayments    map[string]bool // payment txid -> already spent on a session (anti-replay)
	nodeID          string
	nodeWGPubkey    string
	nodeEndpoint    string
	fvpnAsaID       uint64
	pricePerGB      uint64
	maxSessionsPerWallet int
	lastHeartbeat   time.Time

	// per-IP token-bucket rate limiter (abuse mitigation)
	rateMu    sync.Mutex
	buckets   map[string]*bucket
	rateBurst float64
	rateRefill float64 // tokens per second
}

type bucket struct {
	tokens float64
	last   time.Time
}

// allow returns true if the client IP has a token available, consuming one.
func (s *Server) allow(ip string) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	now := time.Now()
	b, ok := s.buckets[ip]
	if !ok {
		b = &bucket{tokens: s.rateBurst, last: now}
		s.buckets[ip] = b
	}
	// refill
	b.tokens += now.Sub(b.last).Seconds() * s.rateRefill
	if b.tokens > s.rateBurst {
		b.tokens = s.rateBurst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func New(
	wgController wg.Controller,
	registryClient *registry.Client,
	pool *ippool.IPPool,
	nodeID string,
	nodeWGPubkey string,
	nodeEndpoint string,
	fvpnAsaID uint64,
	pricePerGB uint64,
) *Server {
	return &Server{
		wgController:        wgController,
		registryClient:      registryClient,
		ipPool:              pool,
		challenges:          make(map[string]Challenge),
		sessions:            make(map[string]SessionData),
		usedPayments:        make(map[string]bool),
		nodeID:              nodeID,
		nodeWGPubkey:        nodeWGPubkey,
		nodeEndpoint:        nodeEndpoint,
		fvpnAsaID:           fvpnAsaID,
		pricePerGB:          pricePerGB,
		maxSessionsPerWallet: 3,
		lastHeartbeat:       time.Now(),
		buckets:             make(map[string]*bucket),
		rateBurst:           20,  // allow short bursts
		rateRefill:          0.5, // 1 token / 2s ≈ 30/min steady state
	}
}

// paymentAlreadyUsed reports whether txnID has already funded a session.
// Callers holding s.mu (e.g. HandleSession) get race-free access for free;
// this mirrors how s.challenges/s.sessions are already accessed in this file.
func (s *Server) paymentAlreadyUsed(txnID string) bool {
	return s.usedPayments[txnID]
}

// markPaymentUsed records that txnID has funded a session, so it cannot be
// replayed to mint another one. Same locking contract as paymentAlreadyUsed.
func (s *Server) markPaymentUsed(txnID string) {
	s.usedPayments[txnID] = true
}

// Health check endpoint
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats, err := s.wgController.Stats()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"status":             "healthy",
		"wg_iface":           "wg0",
		"peers":              len(stats),
		"registered":         true,
		"last_heartbeat_round": 0,
		"version":            "0.1.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Challenge endpoint
func (s *Server) HandleChallenge(w http.ResponseWriter, r *http.Request) {
	if !s.allow(clientIP(r)) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	wallet := r.URL.Query().Get("wallet")
	if wallet == "" {
		http.Error(w, "wallet parameter required", http.StatusBadRequest)
		return
	}

	// Validate wallet address format
	if _, err := types.DecodeAddress(wallet); err != nil {
		http.Error(w, fmt.Sprintf("invalid wallet address: %v", err), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate challenge nonce
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		http.Error(w, "failed to generate challenge", http.StatusInternalServerError)
		return
	}

	challenge := Challenge{
		Value:   base64.StdEncoding.EncodeToString(nonce),
		Nonce:   nonce,
		Expires: time.Now().Add(2 * time.Minute),
	}

	s.challenges[wallet] = challenge

	response := map[string]interface{}{
		"challenge":    challenge.Value,
		"expires_unix": challenge.Expires.Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Session creation endpoint
type SessionRequest struct {
	WalletAddress string `json:"wallet_address"`
	Challenge     string `json:"challenge"`
	Signature     string `json:"signature"`
	PaymentTxID   string `json:"payment_txid"`
	ClientWGPubkey string `json:"client_wg_pubkey"`
}

type SessionResponse struct {
	SessionID       string   `json:"session_id"`
	InterfaceAddress string  `json:"interface_address"`
	NodeWGPubkey    string   `json:"node_wg_pubkey"`
	NodeEndpoint    string   `json:"node_endpoint"`
	AllowedIPs      []string `json:"allowed_ips"`
	DNS             string   `json:"dns"`
	PersistentKeepalive int  `json:"persistent_keepalive"`
}

func (s *Server) HandleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.allow(clientIP(r)) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	var req SessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Verify challenge
	challenge, exists := s.challenges[req.WalletAddress]
	if !exists || time.Now().After(challenge.Expires) {
		http.Error(w, "challenge expired or not found", http.StatusUnauthorized)
		return
	}

	if challenge.Value != req.Challenge {
		http.Error(w, "challenge mismatch", http.StatusUnauthorized)
		return
	}

	// Verify signature
	addr, err := types.DecodeAddress(req.WalletAddress)
	if err != nil {
		http.Error(w, "invalid wallet address", http.StatusBadRequest)
		return
	}

	sigBytes, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		http.Error(w, "invalid signature encoding", http.StatusBadRequest)
		return
	}

	// Algorand address contains encoded public key; extract and verify
	pubKeyBytes := addr[:]
	if len(pubKeyBytes) < 32 || len(sigBytes) != 64 {
		http.Error(w, "invalid signature or public key format", http.StatusUnauthorized)
		return
	}

	// Extract the 32-byte ed25519 public key from the address
	publicKey := ed25519.PublicKey(pubKeyBytes[:32])
	if !ed25519.Verify(publicKey, challenge.Nonce, sigBytes) {
		http.Error(w, "signature verification failed", http.StatusUnauthorized)
		return
	}

	// Verify payment
	valid, err := s.registryClient.VerifyPayment(req.PaymentTxID, s.fvpnAsaID, s.pricePerGB)
	if err != nil || !valid {
		http.Error(w, fmt.Sprintf("payment verification failed: %v", err), http.StatusPaymentRequired)
		return
	}

	// Reject replayed payments: a confirmed on-chain payment is otherwise
	// reusable verbatim by anyone who observes the txid (it stays valid and
	// confirmed for as long as VerifyPayment's lookup window covers), which
	// would let one payment mint sessions repeatedly instead of just once.
	if s.paymentAlreadyUsed(req.PaymentTxID) {
		http.Error(w, "payment already used for a session", http.StatusPaymentRequired)
		return
	}

	// Check per-wallet session cap
	walletSessions := 0
	for _, session := range s.sessions {
		if session.Wallet == req.WalletAddress {
			walletSessions++
		}
	}
	if walletSessions >= s.maxSessionsPerWallet {
		http.Error(w, fmt.Sprintf("max sessions per wallet reached: %d", s.maxSessionsPerWallet), http.StatusTooManyRequests)
		return
	}

	// Allocate IP
	clientIP, err := s.ipPool.Allocate()
	if err != nil {
		http.Error(w, fmt.Sprintf("no capacity: %v", err), http.StatusServiceUnavailable)
		return
	}

	// Add WireGuard peer
	if err := s.wgController.AddPeer(req.ClientWGPubkey, []string{clientIP}); err != nil {
		s.ipPool.Free(clientIP)
		http.Error(w, fmt.Sprintf("failed to add peer: %v", err), http.StatusInternalServerError)
		return
	}

	// Create a unique random session ID
	sidBytes := make([]byte, 16)
	if _, err := rand.Read(sidBytes); err != nil {
		_ = s.wgController.RemovePeer(req.ClientWGPubkey)
		_ = s.ipPool.Free(clientIP)
		http.Error(w, "failed to create session id", http.StatusInternalServerError)
		return
	}
	sessionID := base64.RawURLEncoding.EncodeToString(sidBytes)

	sessionData := SessionData{
		ID:          sessionID,
		Wallet:      req.WalletAddress,
		ClientPubKey: req.ClientWGPubkey,
		IPAddress:   clientIP,
		CreatedAt:   time.Now(),
	}

	s.sessions[sessionID] = sessionData

	// Commit the payment as spent only now that the session actually exists,
	// so a transient failure earlier (e.g. AddPeer) doesn't burn a valid
	// payment without granting a session.
	s.markPaymentUsed(req.PaymentTxID)

	// Remove challenge
	delete(s.challenges, req.WalletAddress)

	response := SessionResponse{
		SessionID:        sessionID,
		InterfaceAddress: clientIP,
		NodeWGPubkey:     s.nodeWGPubkey,
		NodeEndpoint:     s.nodeEndpoint,
		AllowedIPs:       []string{"0.0.0.0/1", "128.0.0.0/1"},
		DNS:              "1.1.1.1",
		PersistentKeepalive: 25,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Session deletion endpoint
func (s *Server) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Path[len("/session/"):]
	if sessionID == "" {
		http.Error(w, "session ID required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Remove WireGuard peer
	if err := s.wgController.RemovePeer(session.ClientPubKey); err != nil {
		http.Error(w, fmt.Sprintf("failed to remove peer: %v", err), http.StatusInternalServerError)
		return
	}

	// Free IP
	if err := s.ipPool.Free(session.IPAddress); err != nil {
		fmt.Printf("warning: failed to free IP %s: %v\n", session.IPAddress, err)
	}

	// Delete session
	delete(s.sessions, sessionID)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.HandleHealth)
	mux.HandleFunc("/challenge", s.HandleChallenge)
	mux.HandleFunc("/session", s.HandleSession)
	mux.HandleFunc("/session/", s.HandleDeleteSession)
}
