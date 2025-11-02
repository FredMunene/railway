// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "forge-std/console2.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

import "src/utils/SeedConstants.sol";
import "src/USDStablecoin.sol";
import "src/CountryToken.sol";
import "src/UserRegistry.sol";
import "src/ComplianceManager.sol";
import "src/MintEscrow.sol";

contract DeployFiatRails is Script {
    struct DeploymentResult {
        address usdStablecoin;
        address countryToken;
        address userRegistry;
        address complianceManager;
        address mintEscrow;
    }

    function run() external {
        uint256 deployerKey = vm.envUint("DEPLOYER_PRIVATE_KEY");
        address deployer = vm.addr(deployerKey);

        address admin = deployer;
        address executor = deployer;

        admin = _tryEnvAddress("ADMIN_ADDRESS", admin);
        executor = _tryEnvAddress("EXECUTOR_ADDRESS", executor);

        vm.startBroadcast(deployerKey);

        USDStablecoin stablecoin = new USDStablecoin();
        CountryToken countryToken = new CountryToken();

        UserRegistry registry = new UserRegistry(admin);

        ComplianceManager implementation = new ComplianceManager();
        bytes memory initData = abi.encodeWithSelector(ComplianceManager.initialize.selector, admin, address(registry));
        ERC1967Proxy proxy = new ERC1967Proxy(address(implementation), initData);
        ComplianceManager complianceManager = ComplianceManager(payable(address(proxy)));

        registry.grantRole(registry.MANAGER_ROLE(), address(complianceManager));

        MintEscrow mintEscrow = new MintEscrow(admin);
        mintEscrow.setStablecoin(address(stablecoin));
        mintEscrow.setUserRegistry(address(registry));

        bytes32 countryCode = _toPaddedBytes32(SeedConstants.COUNTRY_TOKEN_SYMBOL);
        mintEscrow.setCountryToken(countryCode, address(countryToken));
        mintEscrow.setExecutor(executor, true);

        countryToken.grantRole(countryToken.MINTER_ROLE(), address(mintEscrow));

        vm.stopBroadcast();

        DeploymentResult memory deployment = DeploymentResult({
            usdStablecoin: address(stablecoin),
            countryToken: address(countryToken),
            userRegistry: address(registry),
            complianceManager: address(complianceManager),
            mintEscrow: address(mintEscrow)
        });

        _logDeployment(admin, executor, deployer, deployment);
        _writeDeploymentFile(admin, executor, deployer, deployment);
    }

    function _tryEnvAddress(string memory key, address fallbackAddr) private returns (address) {
        try vm.envAddress(key) returns (address value) {
            if (value != address(0)) {
                return value;
            }
        } catch {}
        return fallbackAddr;
    }

    function _toPaddedBytes32(string memory value) private pure returns (bytes32 result) {
        bytes memory raw = bytes(value);
        require(raw.length <= 32, "DeployFiatRails: string too long");
        assembly {
            result := mload(add(raw, 32))
        }
    }

    function _logDeployment(
        address admin,
        address executor,
        address deployer,
        DeploymentResult memory deployment
    ) private view {
        console2.log("FiatRails deployment summary");
        console2.log("---------------------------");
        console2.log("chainId:", block.chainid);
        console2.log("deployer:", deployer);
        console2.log("admin:", admin);
        console2.log("executor:", executor);
        console2.log("USDStablecoin:", deployment.usdStablecoin);
        console2.log("CountryToken:", deployment.countryToken);
        console2.log("UserRegistry:", deployment.userRegistry);
        console2.log("ComplianceManager:", deployment.complianceManager);
        console2.log("MintEscrow:", deployment.mintEscrow);
    }

    function _writeDeploymentFile(
        address admin,
        address executor,
        address deployer,
        DeploymentResult memory deployment
    ) private {
        string memory json = string(
            abi.encodePacked(
                "{",
                "\"chainId\":", vm.toString(block.chainid), ",",
                "\"deployer\":\"", vm.toString(deployer), "\",",
                "\"admin\":\"", vm.toString(admin), "\",",
                "\"executor\":\"", vm.toString(executor), "\",",
                "\"contracts\":{",
                "\"USDStablecoin\":\"", vm.toString(deployment.usdStablecoin), "\",",
                "\"CountryToken\":\"", vm.toString(deployment.countryToken), "\",",
                "\"UserRegistry\":\"", vm.toString(deployment.userRegistry), "\",",
                "\"ComplianceManager\":\"", vm.toString(deployment.complianceManager), "\",",
                "\"MintEscrow\":\"", vm.toString(deployment.mintEscrow), "\"",
                "}",
                "}"
            )
        );

        string memory path = _tryEnvString("DEPLOYMENTS_PATH", "deployments.json");
        vm.writeFile(path, json);
        console2.log("Deployment written to:", path);
    }

    function _tryEnvString(string memory key, string memory fallbackValue) private returns (string memory) {
        try vm.envString(key) returns (string memory value) {
            return bytes(value).length == 0 ? fallbackValue : value;
        } catch {
            return fallbackValue;
        }
    }
}
