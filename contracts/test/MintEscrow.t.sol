// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "src/MintEscrow.sol";
import "src/USDStablecoin.sol";
import "src/CountryToken.sol";
import "src/UserRegistry.sol";
import "src/utils/SeedConstants.sol";
import "@openzeppelin/contracts/access/IAccessControl.sol";

contract MintEscrowTest is Test {
    USDStablecoin internal stablecoin;
    CountryToken internal countryToken;
    UserRegistry internal registry;
    MintEscrow internal escrow;

    address internal constant ADMIN = address(0xA11CE);
    address internal constant EXECUTOR = address(0xE0);
    address internal constant USER = address(0xCAFE);

    bytes32 internal constant COUNTRY_CODE = bytes32("KES");
    bytes32 internal constant TX_REF = bytes32("MPESA-123");
    uint256 internal constant DEPOSIT_AMOUNT = 1_000 ether;

    function setUp() public {
        stablecoin = new USDStablecoin();
        countryToken = new CountryToken();

        vm.startPrank(ADMIN);
        registry = new UserRegistry(ADMIN);
        registry.grantRole(registry.MANAGER_ROLE(), ADMIN);
        registry.grantRole(registry.COMPLIANCE_OFFICER_ROLE(), ADMIN);
        vm.stopPrank();

        escrow = new MintEscrow(ADMIN);

        vm.startPrank(ADMIN);
        escrow.setStablecoin(address(stablecoin));
        escrow.setUserRegistry(address(registry));
        escrow.setCountryToken(COUNTRY_CODE, address(countryToken));
        escrow.setExecutor(EXECUTOR, true);
        vm.stopPrank();

        countryToken.grantRole(countryToken.MINTER_ROLE(), address(escrow));

        // Fund user and approve escrow
        stablecoin.mint(USER, DEPOSIT_AMOUNT * 20);
        vm.prank(USER);
        stablecoin.approve(address(escrow), type(uint256).max);

        vm.prank(ADMIN);
        registry.updateUser(USER, 50, bytes32("KYC_HASH"), bytes32("KYC"), ADMIN);
    }

    function _submitIntent(bytes32 txRef) internal returns (bytes32 intentId) {
        vm.prank(USER);
        intentId = escrow.submitIntent(DEPOSIT_AMOUNT, COUNTRY_CODE, txRef);
    }

    function test_SubmitIntentStoresState() public {
        uint256 userBalanceBefore = stablecoin.balanceOf(USER);

        bytes32 intentId = _submitIntent(TX_REF);

        assertEq(stablecoin.balanceOf(USER), userBalanceBefore - DEPOSIT_AMOUNT);
        assertEq(stablecoin.balanceOf(address(escrow)), DEPOSIT_AMOUNT);

        IMintEscrow.MintIntent memory intent = escrow.getIntent(intentId);
        assertEq(intent.user, USER);
        assertEq(intent.amount, DEPOSIT_AMOUNT);
        assertEq(intent.countryCode, COUNTRY_CODE);
        assertEq(uint8(intent.status), uint8(IMintEscrow.MintStatus.Pending));

        vm.expectRevert(abi.encodeWithSelector(MintEscrow.TxRefAlreadyConsumed.selector, TX_REF));
        _submitIntent(TX_REF);
    }

    function test_ExecuteMintCompliant() public {
        bytes32 intentId = _submitIntent(TX_REF);

        vm.prank(EXECUTOR);
        escrow.executeMint(intentId);

        assertEq(countryToken.balanceOf(USER), DEPOSIT_AMOUNT);
        assertEq(uint8(escrow.getIntentStatus(intentId)), uint8(IMintEscrow.MintStatus.Executed));
    }

    function test_ExecuteMintRevertsWhenNotCompliant() public {
        bytes32 intentId = _submitIntent(TX_REF);

        vm.prank(ADMIN);
        registry.updateUser(USER, SeedConstants.MAX_RISK_SCORE + 1, bytes32("KYC_HASH"), bytes32("KYC"), ADMIN);

        vm.prank(EXECUTOR);
        vm.expectRevert(IMintEscrow.UserNotCompliant.selector);
        escrow.executeMint(intentId);

        assertEq(uint8(escrow.getIntentStatus(intentId)), uint8(IMintEscrow.MintStatus.Pending));
    }

    function test_RefundIntentReturnsFunds() public {
        bytes32 intentId = _submitIntent(TX_REF);
        uint256 escrowBalanceBefore = stablecoin.balanceOf(address(escrow));

        vm.prank(EXECUTOR);
        escrow.refundIntent(intentId, "user failed kyc");

        assertEq(stablecoin.balanceOf(USER), DEPOSIT_AMOUNT * 20);
        assertEq(stablecoin.balanceOf(address(escrow)), escrowBalanceBefore - DEPOSIT_AMOUNT);
        assertEq(uint8(escrow.getIntentStatus(intentId)), uint8(IMintEscrow.MintStatus.Refunded));
    }

    function test_DailyLimitEnforced() public {
        for (uint256 i = 0; i < 10; i++) {
            bytes32 txRef = bytes32(uint256(uint160(i + 1)));
            bytes32 intentId = _submitIntent(txRef);
            vm.prank(EXECUTOR);
            escrow.executeMint(intentId);
        }

        bytes32 txRefOverflow = bytes32(uint256(uint160(999)));
        bytes32 overflowIntent = _submitIntent(txRefOverflow);

        vm.prank(EXECUTOR);
        vm.expectRevert(abi.encodeWithSelector(MintEscrow.DailyLimitExceeded.selector, DEPOSIT_AMOUNT, 0));
        escrow.executeMint(overflowIntent);
    }
}
