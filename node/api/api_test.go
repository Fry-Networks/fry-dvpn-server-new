package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fry-Foundation/fry-dvpn-server-new/node/ippool"
	"github.com/Fry-Foundation/fry-dvpn-server-new/node/wg"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
)

func TestHandleHealth(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)

	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	if response["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", response["status"])
	}

	if response["wg_iface"] != "wg0" {
		t.Errorf("expected wg_iface wg0, got %v", response["wg_iface"])
	}
}

func TestHandleChallenge(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	// Generate a valid Algorand address
	account := crypto.GenerateAccount()
	wallet := account.Address.String()

	req := httptest.NewRequest("GET", "/challenge?wallet="+wallet, nil)
	w := httptest.NewRecorder()

	server.HandleChallenge(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	if response["challenge"] == nil {
		t.Errorf("challenge is nil")
	}

	if response["expires_unix"] == nil {
		t.Errorf("expires_unix is nil")
	}
}

func TestHandleChallengeInvalidWallet(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	req := httptest.NewRequest("GET", "/challenge?wallet=invalid", nil)
	w := httptest.NewRecorder()

	server.HandleChallenge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChallengeNoWallet(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	req := httptest.NewRequest("GET", "/challenge", nil)
	w := httptest.NewRecorder()

	server.HandleChallenge(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleDeleteSessionMethodCheck(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	req := httptest.NewRequest("GET", "/session/test", nil)
	w := httptest.NewRecorder()

	server.HandleDeleteSession(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSessionMethodCheck(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	req := httptest.NewRequest("GET", "/session", nil)
	w := httptest.NewRecorder()

	server.HandleSession(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSessionInvalidJSON(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	body := bytes.NewReader([]byte("invalid json"))
	req := httptest.NewRequest("POST", "/session", body)
	w := httptest.NewRecorder()

	server.HandleSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestServerInterfaceImplementation(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)

	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	// Verify that server is not nil
	if server == nil {
		t.Errorf("server is nil")
	}

	// Verify that server has required fields
	if server.nodeID == "" {
		t.Errorf("nodeID is empty")
	}

	if server.nodeWGPubkey == "" {
		t.Errorf("nodeWGPubkey is empty")
	}
}

func TestRegisterHandlers(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	mux := http.NewServeMux()
	server.RegisterHandlers(mux)

	// Test that handlers are registered by making requests
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected health endpoint to be registered, got status %d", w.Code)
	}
}

func TestChallengeNonceGeneration(t *testing.T) {
	mock := wg.NewMock("10.7.0.1/24", "nodePublicKey")
	pool, _ := ippool.New("10.7.0.0/24", true)
	server := New(mock, nil, pool, "testNodeID", "nodePublicKey", "localhost:51820", 2485198745, 1000000)

	account := crypto.GenerateAccount()
	wallet := account.Address.String()

	// Generate two challenges
	req1 := httptest.NewRequest("GET", "/challenge?wallet="+wallet, nil)
	w1 := httptest.NewRecorder()
	server.HandleChallenge(w1, req1)

	var response1 map[string]interface{}
	json.NewDecoder(w1.Body).Decode(&response1)
	challenge1 := response1["challenge"].(string)

	req2 := httptest.NewRequest("GET", "/challenge?wallet="+wallet, nil)
	w2 := httptest.NewRecorder()
	server.HandleChallenge(w2, req2)

	var response2 map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&response2)
	challenge2 := response2["challenge"].(string)

	// Challenges should be different
	if challenge1 == challenge2 {
		t.Errorf("consecutive challenges should be different")
	}

	// Verify they're valid base64
	_, err1 := base64.StdEncoding.DecodeString(challenge1)
	_, err2 := base64.StdEncoding.DecodeString(challenge2)

	if err1 != nil || err2 != nil {
		t.Errorf("challenges should be valid base64")
	}
}
