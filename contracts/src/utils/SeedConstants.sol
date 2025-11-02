// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title SeedConstants
 * @notice Houses immutable configuration derived from `seed.json`.
 * @dev Centralizing these values keeps contracts aligned with the project seed.
 */
library SeedConstants {
    // Stablecoin metadata
    string internal constant STABLECOIN_NAME = "Dai Stablecoin";
    string internal constant STABLECOIN_SYMBOL = "DAI";
    uint8 internal constant STABLECOIN_DECIMALS = 18;

    // Country token metadata
    string internal constant COUNTRY_TOKEN_NAME = "Kenya Shilling Token";
    string internal constant COUNTRY_TOKEN_SYMBOL = "KES";
    uint8 internal constant COUNTRY_TOKEN_DECIMALS = 18;

    // Compliance configuration
    uint8 internal constant MAX_RISK_SCORE = 91;
    bool internal constant REQUIRE_ATTESTATION = true;
    uint256 internal constant MIN_ATTESTATION_AGE = 0;

    // Chain configuration
    uint256 internal constant CHAIN_ID = 31430;
    uint256 internal constant BLOCK_TIME_SECONDS = 2;

    // Limits (expressed in wei)
    uint256 internal constant MIN_MINT_AMOUNT = 1e18;
    uint256 internal constant MAX_MINT_AMOUNT = 1_000e18;
    uint256 internal constant DAILY_MINT_LIMIT = 10_000e18;
}
