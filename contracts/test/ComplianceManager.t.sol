// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "src/ComplianceManager.sol";
import "src/UserRegistry.sol";
import "@openzeppelin/contracts/access/IAccessControl.sol";
import "@openzeppelin/contracts/proxy/ERC1967/ERC1967Proxy.sol";

contract ComplianceManagerTest is Test {
    ComplianceManager internal manager;
    UserRegistry internal registry;

    address internal constant ADMIN = address(0xA11CE);
    address internal constant OFFICER = address(0xBEEF);
    address internal constant USER = address(0xCAFE);

    function setUp() public {
        registry = new UserRegistry(ADMIN);

        ComplianceManager implementation = new ComplianceManager();
        bytes memory initData =
            abi.encodeWithSelector(ComplianceManager.initialize.selector, ADMIN, address(registry));

        vm.prank(ADMIN);
        ERC1967Proxy proxy = new ERC1967Proxy(address(implementation), initData);

        manager = ComplianceManager(address(proxy));

        vm.startPrank(ADMIN);
        registry.grantRole(registry.MANAGER_ROLE(), address(manager));
        manager.grantRole(manager.COMPLIANCE_OFFICER_ROLE(), OFFICER);
        vm.stopPrank();
    }

    function test_InitializeConfiguresRoles() public {
        assertEq(address(manager.userRegistry()), address(registry));
        assertTrue(manager.hasRole(manager.DEFAULT_ADMIN_ROLE(), ADMIN));
        assertTrue(manager.hasRole(manager.ADMIN_ROLE(), ADMIN));
        assertTrue(manager.hasRole(manager.COMPLIANCE_OFFICER_ROLE(), ADMIN));
        assertTrue(manager.hasRole(manager.UPGRADER_ROLE(), ADMIN));
    }

    function test_UpdateUserThroughManager() public {
        bytes32 attHash = keccak256("kyc");
        bytes32 attType = bytes32("KYC");

        vm.expectEmit(true, false, false, true, address(registry));
        emit UserRegistry.UserRiskUpdated(USER, 40, OFFICER, block.timestamp);
        vm.expectEmit(true, true, false, true, address(registry));
        emit UserRegistry.AttestationRecorded(USER, attHash, attType, OFFICER);

        vm.prank(OFFICER);
        manager.updateUser(USER, 40, attHash, attType);

        assertTrue(registry.isCompliant(USER));
    }

    function test_PauseBlocksUpdates() public {
        bytes32 attHash = keccak256("kyc");
        bytes32 attType = bytes32("KYC");

        vm.prank(ADMIN);
        manager.pause();

        vm.prank(OFFICER);
        vm.expectRevert(Pausable.EnforcedPause.selector);
        manager.updateUser(USER, 10, attHash, attType);
    }

    function test_SetUserRegistryRequiresAdmin() public {
        address newRegistry = address(0xDEAD);

        vm.expectRevert(
            abi.encodeWithSelector(
                IAccessControl.AccessControlUnauthorizedAccount.selector,
                OFFICER,
                manager.ADMIN_ROLE()
            )
        );
        vm.prank(OFFICER);
        manager.setUserRegistry(newRegistry);

        vm.expectEmit(true, true, false, false, address(manager));
        emit ComplianceManager.UserRegistryUpdated(address(registry), ADMIN);
        vm.prank(ADMIN);
        manager.setUserRegistry(address(registry));
    }
}
