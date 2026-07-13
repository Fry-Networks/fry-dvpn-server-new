"""Deploy NodeRegistry + RewardPool.

Network is chosen by env `DVPN_NETWORK` (default "localnet"). Mainnet deployment is
INTENTIONALLY guarded: it requires DVPN_NETWORK=mainnet AND a funded deployer mnemonic
in DEPLOYER_MNEMONIC, and it will NOT run without DVPN_CONFIRM_MAINNET=yes. This keeps
mainnet deployment a deliberate, manual action (per the project's permission model).

Usage:
  # LocalNet (safe; used by QA):
  uv run --with algokit-utils python scripts/deploy.py
  # Mainnet (manual, deliberate):
  DVPN_NETWORK=mainnet DEPLOYER_MNEMONIC="..." DVPN_CONFIRM_MAINNET=yes \
      uv run --with algokit-utils python scripts/deploy.py

Prints the deployed app ids. Set REGISTRY_APP_ID (node daemon) and VITE_REGISTRY_APP_ID
(client) to the NodeRegistry id it prints.
"""

import os
from pathlib import Path

from algokit_utils import AlgorandClient, AlgoAmount, AppClientMethodCallParams

ART = Path(__file__).parent.parent / "smart_contracts"
NODE_REGISTRY_SPEC = ART / "node_registry" / "artifacts" / "NodeRegistry.arc56.json"
REWARD_POOL_SPEC = ART / "reward_pool" / "artifacts" / "RewardPool.arc56.json"

# Mainnet fVPN. On LocalNet a stand-in ASA is created unless FVPN_ASA_ID is set.
FVPN_ASA_MAINNET = 2485198745


def _deployer(algorand: AlgorandClient, network: str):
    if network == "localnet":
        return algorand.account.localnet_dispenser()
    mnem = os.environ.get("DEPLOYER_MNEMONIC")
    if not mnem:
        raise SystemExit("DEPLOYER_MNEMONIC required for non-localnet deploy")
    return algorand.account.from_mnemonic(mnem)


def main() -> None:
    network = os.environ.get("DVPN_NETWORK", "localnet").lower()

    if network == "mainnet":
        if os.environ.get("DVPN_CONFIRM_MAINNET") != "yes":
            raise SystemExit("refusing mainnet deploy without DVPN_CONFIRM_MAINNET=yes")
        algorand = AlgorandClient.main_net()
    elif network == "testnet":
        algorand = AlgorandClient.test_net()
    else:
        algorand = AlgorandClient.default_localnet()

    deployer = _deployer(algorand, network)
    print(f"network={network} deployer={deployer.address}")

    # ---- NodeRegistry ----
    nr_factory = algorand.client.get_app_factory(
        app_spec=NODE_REGISTRY_SPEC.read_text(), default_sender=deployer.address, default_signer=deployer.signer
    )
    nr_client, _ = nr_factory.send.bare.create()
    print(f"NodeRegistry APP_ID = {nr_client.app_id}   addr = {nr_client.app_address}")

    # ---- RewardPool ----
    rp_factory = algorand.client.get_app_factory(
        app_spec=REWARD_POOL_SPEC.read_text(), default_sender=deployer.address, default_signer=deployer.signer
    )
    rp_client, _ = rp_factory.send.bare.create()
    print(f"RewardPool   APP_ID = {rp_client.app_id}   addr = {rp_client.app_address}")

    # fund reward pool app account for MBR + inner-txn fees
    algorand.account.ensure_funded(rp_client.app_address, deployer.address, AlgoAmount.from_algo(1))

    # configure the reward pool with fVPN
    fvpn = int(os.environ.get("FVPN_ASA_ID", "0"))
    if network in ("mainnet", "testnet"):
        fvpn = FVPN_ASA_MAINNET if network == "mainnet" else fvpn
    if fvpn == 0 and network == "localnet":
        from algokit_utils import AssetCreateParams

        res = algorand.send.asset_create(
            AssetCreateParams(sender=deployer.address, total=10_000_000_000, decimals=6, unit_name="fVPN", asset_name="Fry VPN (localnet)")
        )
        fvpn = res.asset_id
        print(f"created LocalNet stand-in fVPN ASA = {fvpn}")
    if fvpn:
        rp_client.send.call(
            AppClientMethodCallParams(method="configure", args=[fvpn], extra_fee=AlgoAmount.from_micro_algo(1000))
        )
        print(f"RewardPool configured with fVPN asa {fvpn}")

    print("\nDEPLOY_RESULT " + str({
        "network": network,
        "node_registry_app_id": nr_client.app_id,
        "reward_pool_app_id": rp_client.app_id,
        "fvpn_asa_id": fvpn,
    }))


if __name__ == "__main__":
    main()
