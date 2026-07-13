"""Shared LocalNet fixtures for contract tests."""

from pathlib import Path

import pytest
from algokit_utils import AlgorandClient, AlgoAmount

ART = Path(__file__).parent.parent / "smart_contracts"
NODE_REGISTRY_SPEC = ART / "node_registry" / "artifacts" / "NodeRegistry.arc56.json"
REWARD_POOL_SPEC = ART / "reward_pool" / "artifacts" / "RewardPool.arc56.json"


@pytest.fixture(scope="session")
def algorand() -> AlgorandClient:
    return AlgorandClient.default_localnet()


@pytest.fixture(scope="session")
def dispenser(algorand: AlgorandClient):
    return algorand.account.localnet_dispenser()


def fund(algorand: AlgorandClient, dispenser, algos: float = 10.0):
    """Create a fresh funded account."""
    acct = algorand.account.random()
    algorand.account.ensure_funded(acct.address, dispenser.address, AlgoAmount.from_algo(algos))
    return acct


def deploy_node_registry(algorand: AlgorandClient, admin):
    factory = algorand.client.get_app_factory(
        app_spec=NODE_REGISTRY_SPEC.read_text(),
        default_sender=admin.address,
        default_signer=admin.signer,
    )
    client, _ = factory.send.bare.create()
    return client


def deploy_reward_pool(algorand: AlgorandClient, admin):
    factory = algorand.client.get_app_factory(
        app_spec=REWARD_POOL_SPEC.read_text(),
        default_sender=admin.address,
        default_signer=admin.signer,
    )
    client, _ = factory.send.bare.create()
    # fund the app account so it can hold assets / pay inner fees
    algorand.account.ensure_funded(
        client.app_address, admin.address, AlgoAmount.from_algo(1)
    )
    return client
