// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IUserRegistry {
    function MANAGER_ROLE() external view returns (bytes32);

    function updateUser(
        address user,
        uint8 newRiskScore,
        bytes32 attestationHash,
        bytes32 attestationType,
        address actor
    ) external;

    function isCompliant(address user) external view returns (bool);
}
