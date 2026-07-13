// Package registry is the node's client for the on-chain NodeRegistry application.
// It builds, signs, and submits real Algorand transactions (via the Atomic
// Transaction Composer) so the node actually registers, heartbeats, updates and
// deregisters on-chain — this is what makes discovery decentralized. It also
// verifies inbound fVPN payments before the node provisions a peer.
package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/abi"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/algorand/go-algorand-sdk/v2/transaction"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// ABI method signatures of NodeRegistry (must match the PuyaPy contract).
const (
	sigRegister   = "register_node(pay,byte[32],string,string,uint32,uint64)void"
	sigUpdate     = "update_node(string,string,uint32,uint64)void"
	sigHeartbeat  = "heartbeat(uint64,uint32)void"
	sigSetStatus  = "set_status(uint8)void"
	sigDeregister = "deregister_node()void"

	defaultMBR = uint64(200_000) // microALGO paid for the node's registry box
)

// Client talks to the NodeRegistry app and signs with the node's account.
type Client struct {
	algod     *algod.Client
	appID     uint64
	appAddr   types.Address
	fvpnASA   uint64
	account   crypto.Account
	signer    transaction.TransactionSigner
	waitRound uint64
}

// New creates a registry client. account holds the node's private key for signing.
func New(algodURL, algodToken string, appID, fvpnASA uint64, account crypto.Account) (*Client, error) {
	c, err := algod.MakeClient(algodURL, algodToken)
	if err != nil {
		return nil, fmt.Errorf("make algod client: %w", err)
	}
	appAddr := crypto.GetApplicationAddress(appID)
	return &Client{
		algod:     c,
		appID:     appID,
		appAddr:   appAddr,
		fvpnASA:   fvpnASA,
		account:   account,
		signer:    transaction.BasicAccountTransactionSigner{Account: account},
		waitRound: 4,
	}, nil
}

func (c *Client) suggestedParams() (types.SuggestedParams, error) {
	return c.algod.SuggestedParams().Do(context.Background())
}

func (c *Client) method(sig string) (abi.Method, error) {
	return abi.MethodFromSignature(sig)
}

// boxRefs declares this node's box (keyed by its 32-byte public key) so the AVM
// permits the app call to read/write it. Required for every method that touches
// the node's NodeRecord box; the Go ATC does not auto-populate these.
func (c *Client) boxRefs() []types.AppBoxReference {
	return []types.AppBoxReference{{AppID: c.appID, Name: c.account.Address[:]}}
}

func (c *Client) execute(atc transaction.AtomicTransactionComposer) (string, error) {
	res, err := atc.Execute(c.algod, context.Background(), c.waitRound)
	if err != nil {
		return "", err
	}
	if len(res.TxIDs) == 0 {
		return "", fmt.Errorf("no txid returned")
	}
	return res.TxIDs[len(res.TxIDs)-1], nil
}

// RegisterNode registers this node on-chain, paying the box MBR in a grouped payment.
func (c *Client) RegisterNode(wgPubkeyB64, endpoint, region string, capacityMbps uint32, pricePerGB uint64) (string, error) {
	wg, err := base64.StdEncoding.DecodeString(wgPubkeyB64)
	if err != nil || len(wg) != 32 {
		return "", fmt.Errorf("wg pubkey must be 32 base64 bytes: %w", err)
	}
	sp, err := c.suggestedParams()
	if err != nil {
		return "", err
	}
	m, err := c.method(sigRegister)
	if err != nil {
		return "", err
	}
	payTxn, err := transaction.MakePaymentTxn(c.account.Address.String(), c.appAddr.String(), defaultMBR, nil, "", sp)
	if err != nil {
		return "", fmt.Errorf("build mbr payment: %w", err)
	}
	var wg32 [32]byte
	copy(wg32[:], wg)

	var atc transaction.AtomicTransactionComposer
	err = atc.AddMethodCall(transaction.AddMethodCallParams{
		AppID:  c.appID,
		Method: m,
		MethodArgs: []interface{}{
			transaction.TransactionWithSigner{Txn: payTxn, Signer: c.signer},
			wg32, endpoint, region, capacityMbps, pricePerGB,
		},
		Sender:          c.account.Address,
		SuggestedParams: sp,
		OnComplete:      types.NoOpOC,
		Signer:          c.signer,
		BoxReferences:   c.boxRefs(),
	})
	if err != nil {
		return "", fmt.Errorf("compose register: %w", err)
	}
	return c.execute(atc)
}

// Heartbeat submits a Proof-of-Connectivity heartbeat with real metered bytes.
func (c *Client) Heartbeat(bytesServedDelta uint64, activeSessions uint32) (string, error) {
	return c.simpleCall(sigHeartbeat, []interface{}{bytesServedDelta, activeSessions}, 0)
}

// UpdateNode updates this node's advertised fields.
func (c *Client) UpdateNode(endpoint, region string, capacityMbps uint32, pricePerGB uint64) (string, error) {
	return c.simpleCall(sigUpdate, []interface{}{endpoint, region, capacityMbps, pricePerGB}, 0)
}

// SetStatus marks the node active(1)/draining(2)/inactive(0).
func (c *Client) SetStatus(status uint8) (string, error) {
	return c.simpleCall(sigSetStatus, []interface{}{status}, 0)
}

// Deregister removes this node's record; extra fee covers the inner MBR refund txn.
func (c *Client) Deregister() (string, error) {
	return c.simpleCall(sigDeregister, []interface{}{}, 1000)
}

func (c *Client) simpleCall(sig string, args []interface{}, extraFee uint64) (string, error) {
	sp, err := c.suggestedParams()
	if err != nil {
		return "", err
	}
	if extraFee > 0 {
		sp.FlatFee = true
		sp.Fee = types.MicroAlgos(uint64(sp.MinFee) + extraFee)
		if sp.Fee == 0 {
			sp.Fee = types.MicroAlgos(1000 + extraFee)
		}
	}
	m, err := c.method(sig)
	if err != nil {
		return "", err
	}
	var atc transaction.AtomicTransactionComposer
	err = atc.AddMethodCall(transaction.AddMethodCallParams{
		AppID:           c.appID,
		Method:          m,
		MethodArgs:      args,
		Sender:          c.account.Address,
		SuggestedParams: sp,
		OnComplete:      types.NoOpOC,
		Signer:          c.signer,
		BoxReferences:   c.boxRefs(),
	})
	if err != nil {
		return "", fmt.Errorf("compose %s: %w", sig, err)
	}
	return c.execute(atc)
}

// VerifyPayment confirms txnID is a confirmed fVPN asset-transfer to this node for
// at least minAmount. Uses algod's pending/recently-confirmed transaction info,
// which retains the txn for the short window between the client's payment and its
// /session call.
func (c *Client) VerifyPayment(txnID string, fvpnAsaID, minAmount uint64) (bool, error) {
	if txnID == "" {
		return false, fmt.Errorf("empty transaction id")
	}
	if fvpnAsaID == 0 {
		fvpnAsaID = c.fvpnASA
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, _, err := c.algod.PendingTransactionInformation(txnID).Do(ctx)
	if err != nil {
		return false, fmt.Errorf("lookup payment: %w", err)
	}
	if info.ConfirmedRound == 0 {
		return false, fmt.Errorf("payment unconfirmed")
	}
	txn := info.Transaction.Txn
	if txn.Type != types.AssetTransferTx {
		return false, fmt.Errorf("not an asset transfer")
	}
	if uint64(txn.XferAsset) != fvpnAsaID {
		return false, fmt.Errorf("wrong asset %d (want %d)", txn.XferAsset, fvpnAsaID)
	}
	if txn.AssetReceiver != c.account.Address {
		return false, fmt.Errorf("payment not to this node")
	}
	if txn.AssetAmount < minAmount {
		return false, fmt.Errorf("payment %d below required %d", txn.AssetAmount, minAmount)
	}
	return true, nil
}

func (c *Client) Close() error { return nil }
