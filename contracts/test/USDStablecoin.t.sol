// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "src/USDStablecoin.sol";
import "src/utils/SeedConstants.sol";

contract USDStablecoinTest is Test {
    USDStablecoin public dai;

    address internal constant USER_A = address(0x100);

    function setUp() public {
        dai = new USDStablecoin();
    }

    function test_InitialState() public {
        assertEq(dai.name(), SeedConstants.STABLECOIN_NAME);
        assertEq(dai.symbol(), SeedConstants.STABLECOIN_SYMBOL);
        assertEq(dai.decimals(), SeedConstants.STABLECOIN_DECIMALS);
    }

    function test_Mint() public {
        uint256 mintAmount = 1000 * 1e18;

        assertEq(dai.balanceOf(USER_A), 0);

        dai.mint(USER_A, mintAmount);

        assertEq(dai.balanceOf(USER_A), mintAmount);
    }
}
