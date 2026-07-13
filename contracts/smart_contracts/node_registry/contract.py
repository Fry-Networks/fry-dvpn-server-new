"""Fry dVPN — NodeRegistry.

Decentralized, on-chain registry of active VPN miner nodes. There is no central
discovery API: consumer clients read the boxes of this application directly from
any public algod/indexer to find nodes. Each node owns exactly one box, keyed by
its Algorand address, and may only mutate its own record. Stale records (no
heartbeat within STALE_ROUNDS) are permissionlessly reap-able, so the registry is
self-cleaning and has no single point of failure.

Compiled with PuyaPy (algorand-python / algopy).
"""

from typing import Literal

from algopy import (
    Account,
    ARC4Contract,
    BoxMap,
    Global,
    Txn,
    UInt64,
    arc4,
    gtxn,
    itxn,
    subroutine,
)

# WireGuard Curve25519 public key: 32 raw bytes.
WgPubKey = arc4.StaticArray[arc4.Byte, Literal[32]]

# status enum
STATUS_INACTIVE = 0
STATUS_ACTIVE = 1
STATUS_DRAINING = 2

# Box pricing (AVM): 2500 microALGO base + 400 per byte of (key + value).
BOX_FLAT = 2500
BOX_PER_BYTE = 400
# Box key = 32-byte address (no extra prefix; BoxMap key_prefix is empty below).
BOX_KEY_LEN = 32


class NodeRecord(arc4.Struct, kw_only=True):
    """One miner node's advertised record. Read directly by clients."""

    wg_pubkey: WgPubKey
    endpoint: arc4.String        # "host:port" the client dials (public IP/DNS + UDP port)
    region: arc4.String          # e.g. "us-east", "eu-west"
    capacity_mbps: arc4.UInt32   # advertised bandwidth capacity
    price_per_gb: arc4.UInt64    # fVPN microunits per GB served
    last_heartbeat: arc4.UInt64  # Algorand round of last heartbeat (Proof-of-Connectivity)
    cumulative_bytes: arc4.UInt64  # lifetime bytes served
    active_sessions: arc4.UInt32   # current live sessions (load signal)
    status: arc4.UInt8             # 0=inactive, 1=active, 2=draining
    mbr_paid: arc4.UInt64          # microALGO the node paid for this box (refunded on deregister)


class NodeRegistry(ARC4Contract):
    def __init__(self) -> None:
        # admin may only pause / tune params — never edit records or seize funds.
        self.admin = Txn.sender
        self.paused = UInt64(0)
        # rounds without a heartbeat after which a node is considered stale/reapable.
        # ~ 30 min at 2.8s/round on mainnet.
        self.stale_rounds = UInt64(640)
        self.node_count = UInt64(0)
        self.nodes = BoxMap(Account, NodeRecord, key_prefix=b"")

    # ---------- internal helpers ----------

    @subroutine
    def _box_mbr(self, value_len: UInt64) -> UInt64:
        return UInt64(BOX_FLAT) + UInt64(BOX_PER_BYTE) * (UInt64(BOX_KEY_LEN) + value_len)

    @subroutine
    def _assert_active(self) -> None:
        assert self.paused == UInt64(0), "registry paused"

    # ---------- node lifecycle ----------

    @arc4.abimethod
    def register_node(
        self,
        mbr_payment: gtxn.PaymentTransaction,
        wg_pubkey: WgPubKey,
        endpoint: arc4.String,
        region: arc4.String,
        capacity_mbps: arc4.UInt32,
        price_per_gb: arc4.UInt64,
    ) -> None:
        """Create the caller's node record. The caller must pay the box MBR in a
        grouped payment to the application address; it is refunded on deregister."""
        self._assert_active()
        assert Txn.sender not in self.nodes, "node already registered"
        assert endpoint.bytes.length > UInt64(0), "endpoint required"

        record = NodeRecord(
            wg_pubkey=wg_pubkey.copy(),
            endpoint=endpoint,
            region=region,
            capacity_mbps=capacity_mbps,
            price_per_gb=price_per_gb,
            last_heartbeat=arc4.UInt64(Global.round),
            cumulative_bytes=arc4.UInt64(0),
            active_sessions=arc4.UInt32(0),
            status=arc4.UInt8(STATUS_ACTIVE),
            mbr_paid=arc4.UInt64(0),
        )
        required = self._box_mbr(record.bytes.length)
        assert mbr_payment.receiver == Global.current_application_address, "MBR must pay app"
        assert mbr_payment.sender == Txn.sender, "MBR must come from caller"
        assert mbr_payment.amount >= required, "insufficient box MBR"

        record.mbr_paid = arc4.UInt64(mbr_payment.amount)
        self.nodes[Txn.sender] = record.copy()
        self.node_count += UInt64(1)

    @arc4.abimethod
    def update_node(
        self,
        endpoint: arc4.String,
        region: arc4.String,
        capacity_mbps: arc4.UInt32,
        price_per_gb: arc4.UInt64,
    ) -> None:
        """Mutate the caller's own advertised fields."""
        self._assert_active()
        assert Txn.sender in self.nodes, "node not registered"
        assert endpoint.bytes.length > UInt64(0), "endpoint required"
        record = self.nodes[Txn.sender].copy()
        record.endpoint = endpoint
        record.region = region
        record.capacity_mbps = capacity_mbps
        record.price_per_gb = price_per_gb
        record.status = arc4.UInt8(STATUS_ACTIVE)
        self.nodes[Txn.sender] = record.copy()

    @arc4.abimethod
    def heartbeat(self, bytes_served_delta: arc4.UInt64, active_sessions: arc4.UInt32) -> None:
        """Proof-of-Connectivity: refresh liveness and report real metered bytes."""
        self._assert_active()
        assert Txn.sender in self.nodes, "node not registered"
        record = self.nodes[Txn.sender].copy()
        record.last_heartbeat = arc4.UInt64(Global.round)
        record.cumulative_bytes = arc4.UInt64(
            record.cumulative_bytes.native + bytes_served_delta.native
        )
        record.active_sessions = active_sessions
        self.nodes[Txn.sender] = record.copy()

    @arc4.abimethod
    def set_status(self, status: arc4.UInt8) -> None:
        """Node marks itself active/draining/inactive (e.g. before shutdown)."""
        assert Txn.sender in self.nodes, "node not registered"
        assert status.native <= UInt64(STATUS_DRAINING), "bad status"
        record = self.nodes[Txn.sender].copy()
        record.status = status
        self.nodes[Txn.sender] = record.copy()

    @arc4.abimethod
    def deregister_node(self) -> None:
        """Delete the caller's record and refund its box MBR."""
        assert Txn.sender in self.nodes, "node not registered"
        refund = self.nodes[Txn.sender].mbr_paid.native
        del self.nodes[Txn.sender]
        self.node_count -= UInt64(1)
        if refund > UInt64(0):
            itxn.Payment(
                receiver=Txn.sender,
                amount=refund,
                fee=0,
            ).submit()

    @arc4.abimethod
    def reap_stale(self, node: Account) -> None:
        """Permissionless: remove a node whose heartbeat is older than stale_rounds.
        The reaper receives the box MBR as an incentive to keep the registry clean."""
        assert node in self.nodes, "no such node"
        record = self.nodes[node].copy()
        assert Global.round - record.last_heartbeat.native > self.stale_rounds, "not stale"
        refund = record.mbr_paid.native
        del self.nodes[node]
        self.node_count -= UInt64(1)
        if refund > UInt64(0):
            itxn.Payment(
                receiver=Txn.sender,
                amount=refund,
                fee=0,
            ).submit()

    # ---------- read-only helpers (also readable straight from boxes) ----------

    @arc4.abimethod(readonly=True)
    def get_node(self, node: Account) -> NodeRecord:
        assert node in self.nodes, "no such node"
        return self.nodes[node]

    @arc4.abimethod(readonly=True)
    def is_stale(self, node: Account) -> arc4.Bool:
        assert node in self.nodes, "no such node"
        age = Global.round - self.nodes[node].last_heartbeat.native
        return arc4.Bool(age > self.stale_rounds)

    @arc4.abimethod(readonly=True)
    def total_nodes(self) -> UInt64:
        return self.node_count

    # ---------- admin (limited) ----------

    @arc4.abimethod
    def set_paused(self, paused: arc4.Bool) -> None:
        assert Txn.sender == self.admin, "admin only"
        self.paused = UInt64(1) if paused.native else UInt64(0)

    @arc4.abimethod
    def set_stale_rounds(self, rounds: arc4.UInt64) -> None:
        assert Txn.sender == self.admin, "admin only"
        assert rounds.native > UInt64(0), "must be > 0"
        self.stale_rounds = rounds.native
