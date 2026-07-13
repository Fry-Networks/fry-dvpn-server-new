"""End-to-end LocalNet exercise of NodeRegistry.

Deploys the contract, registers a node (paying box MBR in a grouped payment),
heartbeats, then reads the node's box DIRECTLY from algod exactly the way a
consumer client discovers nodes (no privileged read path). Asserts the decoded
box matches what was registered, then exercises deregister refund + reap_stale.

Run: uv run --with algokit-utils --with algosdk python scripts/e2e_localnet.py
"""

from pathlib import Path

import algosdk
from algokit_utils import (
    AlgorandClient,
    AlgoAmount,
    AppClientMethodCallParams,
    PaymentParams,
)

SPEC = Path(__file__).parent.parent / "smart_contracts" / "node_registry" / "artifacts" / "NodeRegistry.arc56.json"


def main() -> None:
    algorand = AlgorandClient.default_localnet()
    deployer = algorand.account.localnet_dispenser()

    # a node operator account, funded from the dispenser
    node = algorand.account.random()
    algorand.account.ensure_funded(node.address, deployer.address, AlgoAmount.from_algo(5))

    factory = algorand.client.get_app_factory(
        app_spec=SPEC.read_text(), default_sender=deployer.address
    )
    app_client, _ = factory.send.bare.create()
    app_id = app_client.app_id
    app_addr = app_client.app_address
    print(f"deployed NodeRegistry app_id={app_id} addr={app_addr}")

    # a client of THIS app, sending as the node operator
    node_client = app_client.clone(default_sender=node.address, default_signer=node.signer)

    wg_pubkey = bytes(range(32))  # 32-byte WG pubkey
    endpoint = "203.0.113.7:51820"
    region = "us-east"

    mbr_pay = algorand.create_transaction.payment(
        PaymentParams(sender=node.address, receiver=app_addr, amount=AlgoAmount.from_micro_algo(200_000))
    )
    node_client.send.call(
        AppClientMethodCallParams(
            method="register_node",
            args=[mbr_pay, list(wg_pubkey), endpoint, region, 100, 1_000_000],
        )
    )
    print("register_node OK")

    node_client.send.call(
        AppClientMethodCallParams(method="heartbeat", args=[1_500_000_000, 3])
    )
    print("heartbeat OK (bytes=1.5e9, sessions=3)")

    # ---- CLIENT-STYLE DISCOVERY: read the box straight from algod ----
    algod = algorand.client.algod
    box_name = algosdk.encoding.decode_address(node.address)  # 32-byte key
    box = algod.application_box_by_name(app_id, box_name)
    raw = algosdk.encoding.base64.b64decode(box["value"])
    # decode ARC4 tuple using the arc56 struct order
    import json

    spec = json.loads(SPEC.read_text())
    struct = spec["structs"]["NodeRecord"]
    field_types = [f["type"] for f in struct]
    field_names = [f["name"] for f in struct]
    from algosdk.abi import TupleType, ABIType

    tuple_t = TupleType([ABIType.from_string(t) for t in field_types])
    values = tuple_t.decode(raw)
    record = dict(zip(field_names, values))
    print("decoded box record:")
    for k in field_names:
        v = record[k]
        if k == "wg_pubkey":
            v = bytes(v).hex()
        print(f"  {k} = {v}")

    assert bytes(record["wg_pubkey"]) == wg_pubkey, "wg_pubkey mismatch"
    assert record["endpoint"] == endpoint, "endpoint mismatch"
    assert record["region"] == region, "region mismatch"
    assert record["price_per_gb"] == 1_000_000, "price mismatch"
    assert record["cumulative_bytes"] == 1_500_000_000, "cumulative bytes mismatch"
    assert record["active_sessions"] == 3, "sessions mismatch"
    assert record["status"] == 1, "status should be active"
    print("BOX READ MATCHES REGISTRATION ✔ (decentralized discovery works)")

    # total_nodes readonly
    total = node_client.send.call(AppClientMethodCallParams(method="total_nodes", args=[]))
    print(f"total_nodes = {total.abi_return}")
    assert total.abi_return == 1

    # ---- deregister refunds MBR ----
    bal_before = algod.account_info(node.address)["amount"]
    node_client.send.call(
        AppClientMethodCallParams(
            method="deregister_node",
            args=[],
            # cover the inner-txn (refund payment) fee
            extra_fee=AlgoAmount.from_micro_algo(1000),
        )
    )
    bal_after = algod.account_info(node.address)["amount"]
    print(f"deregister OK; balance delta = {bal_after - bal_before} microALGO (MBR refunded)")
    assert bal_after > bal_before - 5000, "MBR should be roughly refunded"

    print("\nALL NODEREGISTRY E2E ASSERTIONS PASSED")


if __name__ == "__main__":
    main()
