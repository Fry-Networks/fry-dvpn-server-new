"""Fry dVPN — RewardPool.

Distributes fVPN rewards to miner nodes in proportion to the bandwidth they served
(Proof-of-Connectivity), per epoch. The pool is funded in fVPN; each epoch an admin/
oracle records each node's attested bytes (sourced from the NodeRegistry heartbeats);
on close, each node's share is `epoch_budget * node_bytes / epoch_total_bytes` (integer
floor — the remainder stays in the pool, so the sum of shares can never exceed the
budget). A `committed` counter guarantees the pool never promises more fVPN than it
holds. Nodes then claim their accrued fVPN.

Invariants (CLAUDE.md §15):
- total distributed per epoch <= epoch_budget (floor division; remainder retained)
- epoch_budget <= (pool fVPN balance - already committed)  → never over-distribute
- a node cannot claim more than it accrued; claiming zeroes its balance
- privileged methods assert admin
"""

from algopy import (
    Account,
    ARC4Contract,
    Asset,
    BoxMap,
    Global,
    Txn,
    UInt64,
    arc4,
    gtxn,
    itxn,
    subroutine,
)


class RewardPool(ARC4Contract):
    def __init__(self) -> None:
        self.admin = Txn.sender
        self.fvpn_asa = UInt64(0)
        self.epoch_id = UInt64(0)
        self.epoch_open = UInt64(0)
        self.epoch_budget = UInt64(0)
        self.epoch_total_bytes = UInt64(0)
        self.committed = UInt64(0)  # fVPN micro owed to nodes but not yet claimed
        # per-(epoch,node) attested bytes; per-node claimable fVPN
        self.contrib = BoxMap(arc4.DynamicBytes, UInt64, key_prefix=b"c")
        self.claimable = BoxMap(Account, UInt64, key_prefix=b"k")

    # ---------- helpers ----------

    @subroutine
    def _only_admin(self) -> None:
        assert Txn.sender == self.admin, "admin only"

    @subroutine
    def _pool_balance(self) -> UInt64:
        return Asset(self.fvpn_asa).balance(Global.current_application_address)

    @subroutine
    def _contrib_key(self, node: Account) -> arc4.DynamicBytes:
        # key = epoch_id (8 bytes big-endian) || node public key (32 bytes)
        return arc4.DynamicBytes(arc4.UInt64(self.epoch_id).bytes + node.bytes)

    # ---------- setup ----------

    @arc4.abimethod
    def configure(self, fvpn: Asset) -> None:
        """Set the fVPN asset and opt the pool into it (once)."""
        self._only_admin()
        assert self.fvpn_asa == UInt64(0), "already configured"
        self.fvpn_asa = fvpn.id
        itxn.AssetTransfer(
            xfer_asset=fvpn,
            asset_receiver=Global.current_application_address,
            asset_amount=0,
            fee=0,
        ).submit()

    @arc4.abimethod
    def fund(self, deposit: gtxn.AssetTransferTransaction) -> None:
        """Anyone may add fVPN to the pool via a grouped asset-transfer."""
        assert self.fvpn_asa != UInt64(0), "not configured"
        assert deposit.xfer_asset.id == self.fvpn_asa, "wrong asset"
        assert deposit.asset_receiver == Global.current_application_address, "wrong receiver"
        assert deposit.asset_amount > UInt64(0), "zero deposit"

    # ---------- epoch lifecycle ----------

    @arc4.abimethod
    def start_epoch(self, budget: arc4.UInt64) -> None:
        self._only_admin()
        assert self.epoch_open == UInt64(0), "epoch already open"
        assert budget.native > UInt64(0), "budget must be > 0"
        available = self._pool_balance() - self.committed
        assert budget.native <= available, "budget exceeds uncommitted balance"
        self.epoch_id += UInt64(1)
        self.epoch_open = UInt64(1)
        self.epoch_budget = budget.native
        self.epoch_total_bytes = UInt64(0)

    @arc4.abimethod
    def record_contribution(self, node: Account, bytes_served: arc4.UInt64) -> None:
        """Record a node's attested bytes for the open epoch (admin/oracle)."""
        self._only_admin()
        assert self.epoch_open == UInt64(1), "no open epoch"
        assert bytes_served.native > UInt64(0), "zero contribution"
        key = self._contrib_key(node)
        prior = self.contrib.get(key.copy(), default=UInt64(0))
        self.contrib[key.copy()] = prior + bytes_served.native
        self.epoch_total_bytes += bytes_served.native

    @arc4.abimethod
    def settle(self, node: Account) -> None:
        """Move a node's proportional share of the (open) epoch into its claimable
        balance and consume its contribution. Callable by anyone once contributions
        are recorded; safe to call once per node per epoch."""
        assert self.epoch_open == UInt64(1), "no open epoch"
        assert self.epoch_total_bytes > UInt64(0), "no contributions yet"
        key = self._contrib_key(node)
        assert key.copy() in self.contrib, "nothing to settle for node"
        node_bytes = self.contrib[key.copy()]
        # floor division → sum over nodes <= budget; remainder retained in pool
        share = self.epoch_budget * node_bytes // self.epoch_total_bytes
        del self.contrib[key.copy()]
        if share > UInt64(0):
            self.claimable[node] = self.claimable.get(node, default=UInt64(0)) + share
            self.committed += share

    @arc4.abimethod
    def close_epoch(self) -> None:
        self._only_admin()
        assert self.epoch_open == UInt64(1), "no open epoch"
        self.epoch_open = UInt64(0)
        self.epoch_budget = UInt64(0)
        self.epoch_total_bytes = UInt64(0)

    # ---------- claim ----------

    @arc4.abimethod
    def claim(self) -> None:
        """Node claims its accrued fVPN. Cannot exceed what it accrued."""
        assert Txn.sender in self.claimable, "nothing claimable"
        amount = self.claimable[Txn.sender]
        assert amount > UInt64(0), "nothing claimable"
        assert amount <= self.committed, "accounting invariant"
        del self.claimable[Txn.sender]
        self.committed -= amount
        itxn.AssetTransfer(
            xfer_asset=Asset(self.fvpn_asa),
            asset_receiver=Txn.sender,
            asset_amount=amount,
            fee=0,
        ).submit()

    # ---------- read-only ----------

    @arc4.abimethod(readonly=True)
    def claimable_of(self, node: Account) -> UInt64:
        return self.claimable.get(node, default=UInt64(0))

    @arc4.abimethod(readonly=True)
    def pool_info(self) -> arc4.Tuple[arc4.UInt64, arc4.UInt64, arc4.UInt64, arc4.UInt64]:
        return arc4.Tuple(
            (
                arc4.UInt64(self._pool_balance()),
                arc4.UInt64(self.committed),
                arc4.UInt64(self.epoch_id),
                arc4.UInt64(self.epoch_budget),
            )
        )
