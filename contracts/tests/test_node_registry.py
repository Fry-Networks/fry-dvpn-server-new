"""NodeRegistry contract tests (edge cases per CLAUDE.md §15)."""

import algosdk
import pytest
from algokit_utils import AlgoAmount, AppClientMethodCallParams, PaymentParams
from algosdk.abi import ABIType, TupleType

from conftest import deploy_node_registry, fund

WG = list(range(32))
ENDPOINT = "203.0.113.7:51820"


def _mbr_pay(algorand, sender, app_addr, micro=200_000):
    return algorand.create_transaction.payment(
        PaymentParams(sender=sender.address, receiver=app_addr, amount=AlgoAmount.from_micro_algo(micro))
    )


def _register(algorand, client, node, micro=200_000, price=1_000_000):
    c = client.clone(default_sender=node.address, default_signer=node.signer)
    pay = _mbr_pay(algorand, node, client.app_address, micro)
    c.send.call(
        AppClientMethodCallParams(
            method="register_node",
            args=[pay, WG, ENDPOINT, "us-east", 100, price],
        )
    )
    return c


def _read_box(algorand, client, node):
    algod = algorand.client.algod
    box = algod.application_box_by_name(client.app_id, algosdk.encoding.decode_address(node.address))
    raw = algosdk.encoding.base64.b64decode(box["value"])
    import json
    from conftest import NODE_REGISTRY_SPEC

    struct = json.loads(NODE_REGISTRY_SPEC.read_text())["structs"]["NodeRecord"]
    tup = TupleType([ABIType.from_string(f["type"]) for f in struct])
    vals = tup.decode(raw)
    return dict(zip([f["name"] for f in struct], vals))


def test_register_and_box_read(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    _register(algorand, client, node)
    rec = _read_box(algorand, client, node)
    assert bytes(rec["wg_pubkey"]) == bytes(WG)
    assert rec["endpoint"] == ENDPOINT
    assert rec["region"] == "us-east"
    assert rec["price_per_gb"] == 1_000_000
    assert rec["status"] == 1


def test_double_register_rejected(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    _register(algorand, client, node)
    with pytest.raises(Exception):
        _register(algorand, client, node)


def test_insufficient_mbr_rejected(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    with pytest.raises(Exception):
        _register(algorand, client, node, micro=1000)  # far below box MBR


def test_heartbeat_accumulates(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    c = _register(algorand, client, node)
    c.send.call(AppClientMethodCallParams(method="heartbeat", args=[1000, 2]))
    c.send.call(AppClientMethodCallParams(method="heartbeat", args=[500, 4]))
    rec = _read_box(algorand, client, node)
    assert rec["cumulative_bytes"] == 1500
    assert rec["active_sessions"] == 4


def test_update_only_own_node(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    _register(algorand, client, node)
    stranger = fund(algorand, dispenser)
    sc = client.clone(default_sender=stranger.address, default_signer=stranger.signer)
    with pytest.raises(Exception):  # stranger has no box → "node not registered"
        sc.send.call(
            AppClientMethodCallParams(
                method="update_node", args=["1.1.1.1:51820", "eu-west", 50, 2_000_000]
            )
        )


def test_deregister_refunds_mbr(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    c = _register(algorand, client, node)
    algod = algorand.client.algod
    before = algod.account_info(node.address)["amount"]
    c.send.call(
        AppClientMethodCallParams(
            method="deregister_node", args=[], extra_fee=AlgoAmount.from_micro_algo(1000)
        )
    )
    after = algod.account_info(node.address)["amount"]
    # got the MBR (200000) back minus fees
    assert after > before + 190_000


def test_reap_stale(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    # tighten stale window to 1 round
    client.send.call(AppClientMethodCallParams(method="set_stale_rounds", args=[1]))
    node = fund(algorand, dispenser)
    _register(algorand, client, node)
    # advance a few rounds with dummy self-payments (unique note → distinct txids)
    for i in range(3):
        algorand.send.payment(
            PaymentParams(
                sender=admin.address,
                receiver=admin.address,
                amount=AlgoAmount.from_micro_algo(0),
                note=f"advance-{i}".encode(),
            )
        )
    reaper = fund(algorand, dispenser)
    rc = client.clone(default_sender=reaper.address, default_signer=reaper.signer)
    rc.send.call(
        AppClientMethodCallParams(
            method="reap_stale",
            args=[node.address],
            extra_fee=AlgoAmount.from_micro_algo(1000),
        )
    )
    # node box gone
    with pytest.raises(Exception):
        _read_box(algorand, client, node)


def test_reap_requires_staleness(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    node = fund(algorand, dispenser)
    _register(algorand, client, node)  # fresh heartbeat, default stale 640
    reaper = fund(algorand, dispenser)
    rc = client.clone(default_sender=reaper.address, default_signer=reaper.signer)
    with pytest.raises(Exception):
        rc.send.call(
            AppClientMethodCallParams(
                method="reap_stale", args=[node.address], extra_fee=AlgoAmount.from_micro_algo(1000)
            )
        )


def test_admin_only_pause(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    stranger = fund(algorand, dispenser)
    sc = client.clone(default_sender=stranger.address, default_signer=stranger.signer)
    with pytest.raises(Exception):
        sc.send.call(AppClientMethodCallParams(method="set_paused", args=[True]))
    # admin pauses → register blocked
    client.send.call(AppClientMethodCallParams(method="set_paused", args=[True]))
    node = fund(algorand, dispenser)
    with pytest.raises(Exception):
        _register(algorand, client, node)


def test_total_nodes(algorand, dispenser):
    admin = fund(algorand, dispenser)
    client = deploy_node_registry(algorand, admin)
    for _ in range(3):
        _register(algorand, client, fund(algorand, dispenser))
    res = client.send.call(AppClientMethodCallParams(method="total_nodes", args=[]))
    assert res.abi_return == 3
