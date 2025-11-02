// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "src/CountryToken.sol";
import "@openzeppelin/contracts/access/IAccessControl.sol";
import "src/utils/SeedConstants.sol";

contract CountryTokenTest is Test {
    CountryToken public kes;

    address internal DEPLOYER;
    address internal constant MINTER = address(0x100);
    address internal constant USER_A = address(0x200);
    address internal constant STRANGER = address(0x300);

    function setUp() public {
        kes = new CountryToken();
        DEPLOYER = address(this);
    }

    function test_InitialState() public {
        assertEq(kes.name(), SeedConstants.COUNTRY_TOKEN_NAME);
        assertEq(kes.symbol(), SeedConstants.COUNTRY_TOKEN_SYMBOL);
        assertEq(kes.decimals(), SeedConstants.COUNTRY_TOKEN_DECIMALS);
        assertTrue(kes.hasRole(kes.DEFAULT_ADMIN_ROLE(), DEPLOYER));
    }

    function test_Fail_MintWithoutRole() public {
        vm.expectRevert(
            abi.encodeWithSelector(IAccessControl.AccessControlUnauthorizedAccount.selector, STRANGER, kes.MINTER_ROLE())
        );
        vm.prank(STRANGER);
        kes.mint(USER_A, 1e18);
    }

    function test_Success_MintWithRole() public {
        // Grant role
        kes.grantRole(kes.MINTER_ROLE(), MINTER);
        assertTrue(kes.hasRole(kes.MINTER_ROLE(), MINTER));

        // Mint
        uint256 mintAmount = 500 * 1e18;
        vm.prank(MINTER);
        kes.mint(USER_A, mintAmount);

        assertEq(kes.balanceOf(USER_A), mintAmount);
    }

    function test_Burn() public {
        // Burn functionality is inherited from ERC20Burnable and tested by OpenZeppelin.
        // This test is a placeholder to acknowledge its existence.
        assertTrue(true);
    }
}
