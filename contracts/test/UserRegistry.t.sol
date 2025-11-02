// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "src/UserRegistry.sol";
import "src/utils/SeedConstants.sol";
import "@openzeppelin/contracts/access/IAccessControl.sol";

contract UserRegistryTest is Test {
    UserRegistry internal registry;

    address internal constant ADMIN = address(0xA11CE);
    address internal constant MANAGER = address(0xBEEF);
    address internal constant OFFICER = address(0xCAFE);
    address internal constant USER = address(0xD00D);

    function setUp() public {
        vm.startPrank(ADMIN);
        registry = new UserRegistry(ADMIN);
        registry.grantRole(registry.MANAGER_ROLE(), MANAGER);
        registry.grantRole(registry.COMPLIANCE_OFFICER_ROLE(), OFFICER);
        vm.stopPrank();
    }

    function test_UpdateUser_Succeeds() public {
        bytes32 hash = keccak256("att");
        bytes32 attType = bytes32("KYC");

        vm.expectEmit(true, false, false, true, address(registry));
        emit UserRegistry.UserRiskUpdated(USER, 50, OFFICER, block.timestamp);
        vm.expectEmit(true, true, false, true, address(registry));
        emit UserRegistry.AttestationRecorded(USER, hash, attType, OFFICER);
        vm.prank(MANAGER);
        registry.updateUser(USER, 50, hash, attType, OFFICER);

        UserRegistry.UserData memory data = registry.getUser(USER);
        assertEq(data.riskScore, 50);
        assertEq(data.attestationHash, hash);
        assertEq(data.attestationType, attType);
        assertTrue(data.exists);

        assertTrue(registry.isCompliant(USER));
    }

    function test_Fail_UpdateWithoutManagerRole() public {
        vm.expectRevert(
            abi.encodeWithSelector(
                IAccessControl.AccessControlUnauthorizedAccount.selector,
                address(this),
                registry.MANAGER_ROLE()
            )
        );
        registry.updateUser(USER, 10, bytes32("hash"), bytes32("type"), OFFICER);
    }

    function test_Fail_AttestationRequired() public {
        vm.prank(MANAGER);
        vm.expectRevert(UserRegistry.AttestationRequired.selector);
        registry.updateUser(USER, 42, bytes32(0), bytes32("type"), OFFICER);
    }

    function test_Fail_RiskScoreOutOfRange() public {
        vm.prank(MANAGER);
        vm.expectRevert(abi.encodeWithSelector(UserRegistry.RiskScoreOutOfRange.selector, 150));
        registry.updateUser(USER, 150, bytes32("hash"), bytes32("type"), OFFICER);
    }

    function test_ComplianceFalseWhenRiskAboveThreshold() public {
        vm.prank(MANAGER);
        registry.updateUser(USER, SeedConstants.MAX_RISK_SCORE + 1, bytes32("hash"), bytes32("type"), OFFICER);

        assertFalse(registry.isCompliant(USER));
    }

    function test_ComplianceFalseWhenNoRecord() public view {
        assertFalse(registry.isCompliant(USER));
    }
}
