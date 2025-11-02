// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/access/AccessControl.sol";
import {SeedConstants} from "./utils/SeedConstants.sol";

/**
 * @title UserRegistry
 * @notice Stores compliance metadata for users and exposes a query interface.
 */
contract UserRegistry is AccessControl {
    struct UserData {
        uint8 riskScore;
        bytes32 attestationHash;
        bytes32 attestationType;
        uint64 riskUpdatedAt;
        uint64 attestedAt;
        bool exists;
    }

    bytes32 public constant ADMIN_ROLE = keccak256("ADMIN_ROLE");
    bytes32 public constant MANAGER_ROLE = keccak256("MANAGER_ROLE");
    bytes32 public constant COMPLIANCE_OFFICER_ROLE = keccak256("COMPLIANCE_OFFICER_ROLE");

    error InvalidAccount(address account);
    error RiskScoreOutOfRange(uint8 provided);
    error AttestationRequired();

    mapping(address => UserData) private _users;

    event UserRiskUpdated(address indexed user, uint8 newRiskScore, address indexed updatedBy, uint256 timestamp);
    event AttestationRecorded(
        address indexed user,
        bytes32 indexed attestationHash,
        bytes32 attestationType,
        address indexed recordedBy
    );

    constructor(address initialAdmin) {
        if (initialAdmin == address(0)) {
            revert InvalidAccount(initialAdmin);
        }

        _grantRole(DEFAULT_ADMIN_ROLE, initialAdmin);
        _grantRole(ADMIN_ROLE, initialAdmin);

        _setRoleAdmin(ADMIN_ROLE, ADMIN_ROLE);
        _setRoleAdmin(MANAGER_ROLE, ADMIN_ROLE);
        _setRoleAdmin(COMPLIANCE_OFFICER_ROLE, ADMIN_ROLE);
    }

    function updateUser(
        address user,
        uint8 newRiskScore,
        bytes32 attestationHash,
        bytes32 attestationType,
        address actor
    ) external onlyRole(MANAGER_ROLE) {
        if (user == address(0) || actor == address(0)) {
            revert InvalidAccount(user == address(0) ? user : actor);
        }

        if (newRiskScore > 100) {
            revert RiskScoreOutOfRange(newRiskScore);
        }

        if (SeedConstants.REQUIRE_ATTESTATION && attestationHash == bytes32(0)) {
            revert AttestationRequired();
        }

        uint64 currentTimestamp = uint64(block.timestamp);

        UserData storage data = _users[user];
        data.riskScore = newRiskScore;
        data.riskUpdatedAt = currentTimestamp;
        data.exists = true;

        emit UserRiskUpdated(user, newRiskScore, actor, currentTimestamp);

        if (attestationHash != bytes32(0)) {
            data.attestationHash = attestationHash;
            data.attestationType = attestationType;
            data.attestedAt = currentTimestamp;

            emit AttestationRecorded(user, attestationHash, attestationType, actor);
        }
    }

    function getUser(address user) external view returns (UserData memory) {
        return _users[user];
    }

    function isCompliant(address user) external view returns (bool) {
        UserData memory data = _users[user];
        if (!data.exists) {
            return false;
        }

        if (data.riskScore > SeedConstants.MAX_RISK_SCORE) {
            return false;
        }

        if (SeedConstants.REQUIRE_ATTESTATION) {
            if (data.attestationHash == bytes32(0)) {
                return false;
            }

            if (SeedConstants.MIN_ATTESTATION_AGE > 0) {
                if (data.attestedAt == 0) {
                    return false;
                }
                if (block.timestamp - data.attestedAt < SeedConstants.MIN_ATTESTATION_AGE) {
                    return false;
                }
            }
        }

        return true;
    }
}
