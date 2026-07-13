"""RewardPool contract tests — distribution correctness & over-distribution guards."""

import pytest
from algokit_utils import (
    AlgoAmount,
    AppClientMethodCallParams,
    AssetTransferParams,
    AssetCreateParams,
    AssetOptInParams,
)

from conftest import deploy_reward_pool, fund


def _make_fvpn(algorand, creator, total=10_000_000_000):
    """Create a stand-in fVPN ASA on LocalNet (real fVPN 2485198745 is mainnet-only)."""
    res = algorand.send.asset_create(
        AssetCreateParams(sender=creator.address, total=total, decimals=6, unit_name="fVPN", asset_name="Fry VPN")
    )
    return res.asset_id


def _optin(algorand, acct, asa):
    algorand.send.asset_opt_in(AssetOptInParams(sender=acct.address, asset_id=asa))


def _configure_and_fund(algorand, dispenser, admin, client, asa, amount):
    # admin holds the ASA (creator). configure pool → opts in via inner txn.
    client.send.call(
        AppClientMethodCallParams(method="configure", args=[asa], extra_fee=AlgoAmount.from_micro_algo(1000))
    )
    deposit = algorand.create_transaction.asset_transfer(
        AssetTransferParams(sender=admin.address, receiver=client.app_address, asset_id=asa, amount=amount)
    )
    client.send.call(AppClientMethodCallParams(method="fund", args=[deposit]))


def test_configure_and_fund(algorand, dispenser):
    admin = fund(algorand, dispenser)
    asa = _make_fvpn(algorand, admin)
    client = deploy_reward_pool(algorand, admin)
    _configure_and_fund(algorand, dispenser, admin, client, asa, 5_000_000)
    info = client.send.call(AppClientMethodCallParams(method="pool_info", args=[]))
    bal, committed, epoch, budget = info.abi_return
    assert bal == 5_000_000 and committed == 0 and epoch == 0


def test_proportional_distribution(algorand, dispenser):
    admin = fund(algorand, dispenser)
    asa = _make_fvpn(algorand, admin)
    client = deploy_reward_pool(algorand, admin)
    _configure_and_fund(algorand, dispenser, admin, client, asa, 10_000_000)

    n1 = fund(algorand, dispenser)
    n2 = fund(algorand, dispenser)
    _optin(algorand, n1, asa)
    _optin(algorand, n2, asa)

    client.send.call(AppClientMethodCallParams(method="start_epoch", args=[1000]))
    client.send.call(AppClientMethodCallParams(method="record_contribution", args=[n1.address, 3000]))
    client.send.call(AppClientMethodCallParams(method="record_contribution", args=[n2.address, 1000]))
    # settle each: shares = 1000 * 3000/4000 = 750 ; 1000*1000/4000 = 250
    client.send.call(AppClientMethodCallParams(method="settle", args=[n1.address]))
    client.send.call(AppClientMethodCallParams(method="settle", args=[n2.address]))

    assert client.send.call(AppClientMethodCallParams(method="claimable_of", args=[n1.address])).abi_return == 750
    assert client.send.call(AppClientMethodCallParams(method="claimable_of", args=[n2.address])).abi_return == 250

    algod = algorand.client.algod
    c1 = client.clone(default_sender=n1.address, default_signer=n1.signer)
    c1.send.call(AppClientMethodCallParams(method="claim", args=[], extra_fee=AlgoAmount.from_micro_algo(1000)))
    bal1 = next(a for a in algod.account_info(n1.address)["assets"] if a["asset-id"] == asa)["amount"]
    assert bal1 == 750
    # cannot double-claim
    with pytest.raises(Exception):
        c1.send.call(AppClientMethodCallParams(method="claim", args=[], extra_fee=AlgoAmount.from_micro_algo(1000)))


def test_floor_remainder_never_over_distributes(algorand, dispenser):
    admin = fund(algorand, dispenser)
    asa = _make_fvpn(algorand, admin)
    client = deploy_reward_pool(algorand, admin)
    _configure_and_fund(algorand, dispenser, admin, client, asa, 10_000_000)
    n1 = fund(algorand, dispenser)
    n2 = fund(algorand, dispenser)
    n3 = fund(algorand, dispenser)
    client.send.call(AppClientMethodCallParams(method="start_epoch", args=[100]))
    # 1:1:1 over 100 → floor(33) each = 99, remainder 1 stays in pool
    for n in (n1, n2, n3):
        client.send.call(AppClientMethodCallParams(method="record_contribution", args=[n.address, 1]))
    total = 0
    for n in (n1, n2, n3):
        client.send.call(AppClientMethodCallParams(method="settle", args=[n.address]))
        total += client.send.call(AppClientMethodCallParams(method="claimable_of", args=[n.address])).abi_return
    assert total == 99  # <= budget 100, remainder retained
    _, committed, _, _ = client.send.call(AppClientMethodCallParams(method="pool_info", args=[])).abi_return
    assert committed == 99


def test_budget_cannot_exceed_balance(algorand, dispenser):
    admin = fund(algorand, dispenser)
    asa = _make_fvpn(algorand, admin)
    client = deploy_reward_pool(algorand, admin)
    _configure_and_fund(algorand, dispenser, admin, client, asa, 1000)
    with pytest.raises(Exception):
        client.send.call(AppClientMethodCallParams(method="start_epoch", args=[5000]))  # > balance


def test_admin_only(algorand, dispenser):
    admin = fund(algorand, dispenser)
    asa = _make_fvpn(algorand, admin)
    client = deploy_reward_pool(algorand, admin)
    _configure_and_fund(algorand, dispenser, admin, client, asa, 1_000_000)
    stranger = fund(algorand, dispenser)
    sc = client.clone(default_sender=stranger.address, default_signer=stranger.signer)
    with pytest.raises(Exception):
        sc.send.call(AppClientMethodCallParams(method="start_epoch", args=[1000]))
    with pytest.raises(Exception):
        sc.send.call(AppClientMethodCallParams(method="configure", args=[asa]))


def test_two_epochs_committed_tracking(algorand, dispenser):
    admin = fund(algorand, dispenser)
    asa = _make_fvpn(algorand, admin)
    client = deploy_reward_pool(algorand, admin)
    _configure_and_fund(algorand, dispenser, admin, client, asa, 10_000_000)
    n1 = fund(algorand, dispenser)
    _optin(algorand, n1, asa)

    # epoch 1
    client.send.call(AppClientMethodCallParams(method="start_epoch", args=[1000]))
    client.send.call(AppClientMethodCallParams(method="record_contribution", args=[n1.address, 10]))
    client.send.call(AppClientMethodCallParams(method="settle", args=[n1.address]))
    client.send.call(AppClientMethodCallParams(method="close_epoch", args=[]))
    assert client.send.call(AppClientMethodCallParams(method="claimable_of", args=[n1.address])).abi_return == 1000

    # epoch 2 budget must respect committed (1000 already owed)
    _, committed, epoch, _ = client.send.call(AppClientMethodCallParams(method="pool_info", args=[])).abi_return
    assert committed == 1000 and epoch == 1
    client.send.call(AppClientMethodCallParams(method="start_epoch", args=[500]))
    client.send.call(AppClientMethodCallParams(method="record_contribution", args=[n1.address, 10]))
    client.send.call(AppClientMethodCallParams(method="settle", args=[n1.address]))
    assert client.send.call(AppClientMethodCallParams(method="claimable_of", args=[n1.address])).abi_return == 1500
