// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/proxy/utils/Initializable.sol";
import "@openzeppelin/contracts/proxy/utils/UUPSUpgradeable.sol";
import "@openzeppelin/contracts/utils/Pausable.sol";
import "@openzeppelin/contracts/access/AccessControl.sol";
import {IUserRegistry} from "./interfaces/IUserRegistry.sol";

/**
 * @title ComplianceManager
 * @notice Administrative entry point for compliance operations. Implements UUPS upgradeability.
 */
contract ComplianceManager is Initializable, UUPSUpgradeable, Pausable, AccessControl {
    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant COMPLIANCE_OFFICER_ROLE = keccak256("COMPLIANCE_OFFICER_ROLE");
    bytes32 public constant UPGRADER_ROLE = keccak256("UPGRADER_ROLE");

    IUserRegistry public userRegistry;

    error InvalidAddress(address account);

    event UserRegistryUpdated(address indexed newRegistry, address indexed updatedBy);

    /// @custom:oz-upgrades-unsafe-allow constructor
    constructor() {
        _disableInitializers();
    }

    function initialize(address admin, address registry) external initializer {
        if (admin == address(0) || registry == address(0)) {
            revert InvalidAddress(admin == address(0) ? admin : registry);
        }

        userRegistry = IUserRegistry(registry);

        _setRoleAdmin(ADMIN_ROLE, ADMIN_ROLE);
        _setRoleAdmin(COMPLIANCE_OFFICER_ROLE, ADMIN_ROLE);
        _setRoleAdmin(UPGRADER_ROLE, ADMIN_ROLE);

        _grantRole(DEFAULT_ADMIN_ROLE, admin);
        _grantRole(ADMIN_ROLE, admin);
        _grantRole(COMPLIANCE_OFFICER_ROLE, admin);
        _grantRole(UPGRADER_ROLE, admin);
    }

    function pause() external onlyRole(ADMIN_ROLE) {
        _pause();
    }

    function unpause() external onlyRole(ADMIN_ROLE) {
        _unpause();
    }

    function updateUser(
        address user,
        uint8 newRiskScore,
        bytes32 attestationHash,
        bytes32 attestationType
    ) external whenNotPaused onlyRole(COMPLIANCE_OFFICER_ROLE) {
        userRegistry.updateUser(user, newRiskScore, attestationHash, attestationType, msg.sender);
    }

    function setUserRegistry(address registry) external onlyRole(ADMIN_ROLE) {
        if (registry == address(0)) {
            revert InvalidAddress(registry);
        }
        userRegistry = IUserRegistry(registry);
        emit UserRegistryUpdated(registry, msg.sender);
    }

    function _authorizeUpgrade(address newImplementation) internal override onlyRole(UPGRADER_ROLE) {}

    function supportsInterface(bytes4 interfaceId) public view override(AccessControl) returns (bool) {
        return super.supportsInterface(interfaceId);
    }
}
