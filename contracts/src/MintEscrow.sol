// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/utils/Pausable.sol";
import "@openzeppelin/contracts/access/AccessControl.sol";
import {IMintEscrow} from "./IMintEscrow.sol";
import {IUserRegistry} from "./interfaces/IUserRegistry.sol";
import {SeedConstants} from "./utils/SeedConstants.sol";

interface ICountryToken {
    function mint(address to, uint256 amount) external;
}

/**
 * @title MintEscrow
 * @notice Escrow contract that holds USD stablecoin deposits and mints country tokens after compliance checks.
 */
contract MintEscrow is IMintEscrow, AccessControl, Pausable, ReentrancyGuard {
    using SafeERC20 for IERC20;

    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant EXECUTOR_ROLE = keccak256("EXECUTOR_ROLE");

    IERC20 public stablecoin;
    IUserRegistry public userRegistry;

    mapping(bytes32 intentId => MintIntent) private _intents;
    mapping(bytes32 txRef => bool) private _txRefConsumed;
    mapping(bytes32 countryCode => address token) private _countryTokens;
    mapping(uint256 dayBucket => uint256 amount) public dailyMinted;

    error InvalidAddress(address account);
    error StablecoinNotSet();
    error UserRegistryNotSet();
    error DailyLimitExceeded(uint256 requested, uint256 available);
    error CountryTokenNotConfigured(bytes32 countryCode);
    error TxRefAlreadyConsumed(bytes32 txRef);
    error InvalidTxRef();

    event CountryTokenConfigured(bytes32 indexed countryCode, address indexed token, address indexed actor);
    event StablecoinUpdated(address indexed token, address indexed actor);
    event UserRegistryUpdated(address indexed registry, address indexed actor);
    event ExecutorUpdated(address indexed executor, bool granted, address indexed actor);

    constructor(address admin) {
        if (admin == address(0)) {
            revert InvalidAddress(admin);
        }

        _grantRole(DEFAULT_ADMIN_ROLE, admin);
        _grantRole(ADMIN_ROLE, admin);
    }

    /**
     * @notice Configure the USD stablecoin address.
     */
    function setStablecoin(address token) external override onlyRole(ADMIN_ROLE) {
        if (token == address(0)) {
            revert StablecoinNotSet();
        }

        stablecoin = IERC20(token);
        emit StablecoinUpdated(token, msg.sender);
    }

    /**
     * @notice Configure the user registry address.
     */
    function setUserRegistry(address registry) external override onlyRole(ADMIN_ROLE) {
        if (registry == address(0)) {
            revert UserRegistryNotSet();
        }

        userRegistry = IUserRegistry(registry);
        emit UserRegistryUpdated(registry, msg.sender);
    }

    /**
     * @notice Map a country code to its token contract.
     */
    function setCountryToken(bytes32 countryCode, address token) external onlyRole(ADMIN_ROLE) {
        if (token == address(0)) {
            revert CountryTokenNotConfigured(countryCode);
        }
        _countryTokens[countryCode] = token;
        emit CountryTokenConfigured(countryCode, token, msg.sender);
    }

    /**
     * @notice Grant or revoke executor permissions.
     */
    function setExecutor(address executor, bool granted) external onlyRole(ADMIN_ROLE) {
        if (granted) {
            _grantRole(EXECUTOR_ROLE, executor);
        } else {
            _revokeRole(EXECUTOR_ROLE, executor);
        }
        emit ExecutorUpdated(executor, granted, msg.sender);
    }

    /**
     * @notice Pause critical state-changing flows.
     */
    function pause() external onlyRole(ADMIN_ROLE) {
        _pause();
    }

    /**
     * @notice Resume operations.
     */
    function unpause() external onlyRole(ADMIN_ROLE) {
        _unpause();
    }

    function submitIntent(
        uint256 amount,
        bytes32 countryCode,
        bytes32 txRef
    ) external override nonReentrant whenNotPaused returns (bytes32) {
        if (address(stablecoin) == address(0)) {
            revert StablecoinNotSet();
        }
        if (address(userRegistry) == address(0)) {
            revert UserRegistryNotSet();
        }
        if (amount < SeedConstants.MIN_MINT_AMOUNT || amount > SeedConstants.MAX_MINT_AMOUNT) {
            revert InvalidAmount();
        }
        if (countryCode == bytes32(0)) {
            revert InvalidCountryCode();
        }
        if (_countryTokens[countryCode] == address(0)) {
            revert CountryTokenNotConfigured(countryCode);
        }
        if (txRef == bytes32(0)) {
            revert InvalidTxRef();
        }
        if (_txRefConsumed[txRef]) {
            revert TxRefAlreadyConsumed(txRef);
        }

        bytes32 intentId = keccak256(abi.encodePacked(msg.sender, amount, countryCode, txRef));
        if (_intents[intentId].user != address(0)) {
            revert IntentAlreadyExists();
        }

        stablecoin.safeTransferFrom(msg.sender, address(this), amount);

        _txRefConsumed[txRef] = true;

        _intents[intentId] = MintIntent({
            user: msg.sender,
            amount: amount,
            countryCode: countryCode,
            txRef: txRef,
            timestamp: block.timestamp,
            status: MintStatus.Pending
        });

        emit MintIntentSubmitted(intentId, msg.sender, amount, countryCode, txRef);
        return intentId;
    }

    function executeMint(bytes32 intentId) external override nonReentrant whenNotPaused onlyRole(EXECUTOR_ROLE) {
        MintIntent storage intent = _intents[intentId];
        if (intent.user == address(0)) {
            revert IntentNotFound();
        }
        if (intent.status != MintStatus.Pending) {
            revert IntentAlreadyExecuted();
        }
        if (!userRegistry.isCompliant(intent.user)) {
            revert UserNotCompliant();
        }

        address tokenAddr = _countryTokens[intent.countryCode];
        if (tokenAddr == address(0)) {
            revert CountryTokenNotConfigured(intent.countryCode);
        }

        uint256 dayBucket = block.timestamp / 1 days;
        uint256 available = _availableMintingCapacity(dayBucket);
        if (intent.amount > available) {
            revert DailyLimitExceeded(intent.amount, available);
        }

        intent.status = MintStatus.Executed;
        dailyMinted[dayBucket] += intent.amount;

        ICountryToken(tokenAddr).mint(intent.user, intent.amount);

        emit MintExecuted(intentId, intent.user, intent.amount, intent.countryCode, intent.txRef);
    }

    function refundIntent(bytes32 intentId, string calldata reason)
        external
        override
        nonReentrant
        onlyRole(EXECUTOR_ROLE)
    {
        MintIntent storage intent = _intents[intentId];
        if (intent.user == address(0)) {
            revert IntentNotFound();
        }
        if (intent.status != MintStatus.Pending) {
            revert IntentAlreadyExecuted();
        }

        intent.status = MintStatus.Refunded;

        stablecoin.safeTransfer(intent.user, intent.amount);

        emit MintRefunded(intentId, intent.user, intent.amount, reason);
    }

    function getIntent(bytes32 intentId) external view override returns (MintIntent memory) {
        return _intents[intentId];
    }

    function getIntentStatus(bytes32 intentId) external view override returns (MintStatus) {
        return _intents[intentId].status;
    }

    function getCountryToken(bytes32 countryCode) external view returns (address) {
        return _countryTokens[countryCode];
    }

    function _availableMintingCapacity(uint256 dayBucket) internal view returns (uint256) {
        if (SeedConstants.DAILY_MINT_LIMIT <= dailyMinted[dayBucket]) {
            return 0;
        }
        return SeedConstants.DAILY_MINT_LIMIT - dailyMinted[dayBucket];
    }

    function supportsInterface(bytes4 interfaceId) public view override(AccessControl) returns (bool) {
        return super.supportsInterface(interfaceId);
    }
}
