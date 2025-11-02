// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import "@openzeppelin/contracts/access/AccessControl.sol";
import {SeedConstants} from "./utils/SeedConstants.sol";

/**
 * @title CountryToken
 * @author FiatRails Candidate
 * @notice An ERC20 token representing a country-specific currency.
 * Minting is restricted to accounts with the MINTER_ROLE.
 * The contract deployer receives the DEFAULT_ADMIN_ROLE for role management.
 * The name and symbol are derived from the project's seed.json file.
 */
contract CountryToken is ERC20, ERC20Burnable, AccessControl {
    bytes32 public constant MINTER_ROLE = keccak256("MINTER_ROLE");

    constructor() ERC20(SeedConstants.COUNTRY_TOKEN_NAME, SeedConstants.COUNTRY_TOKEN_SYMBOL) {
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
    }

    /**
     * @notice Creates `amount` tokens and assigns them to `to`, increasing the total supply.
     * @dev Emits a {Transfer} event with `from` set to the zero address.
     * Requirements:
     * - The caller must have the `MINTER_ROLE`.
     */
    function mint(address to, uint256 amount) public virtual onlyRole(MINTER_ROLE) {
        _mint(to, amount);
    }

    /// @inheritdoc ERC20
    function decimals() public pure override returns (uint8) {
        return SeedConstants.COUNTRY_TOKEN_DECIMALS;
    }
}
