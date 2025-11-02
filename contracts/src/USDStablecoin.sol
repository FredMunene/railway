// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {SeedConstants} from "./utils/SeedConstants.sol";

/**
 * @title USDStablecoin
 * @author FiatRails Candidate
 * @notice A mock ERC20 stablecoin for testing purposes.
 * It includes a public mint function to allow any address to acquire tokens for test setups.
 * The name and symbol are derived from the project's seed.json file.
 */
contract USDStablecoin is ERC20 {
    constructor() ERC20(SeedConstants.STABLECOIN_NAME, SeedConstants.STABLECOIN_SYMBOL) {}

    /**
     * @notice Mints `amount` tokens to `to`. This is an open function for testing.
     */
    function mint(address to, uint256 amount) public {
        _mint(to, amount);
    }

    /// @inheritdoc ERC20
    function decimals() public pure override returns (uint8) {
        return SeedConstants.STABLECOIN_DECIMALS;
    }
}
