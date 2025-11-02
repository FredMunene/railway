# Debug Log

## 2025-11-02

### Challenge: AccessControl revert mismatch during token negative test
- **Source file:** `contracts/test/CountryToken.t.sol`
- **Issue summary:** The test `test_Fail_MintWithoutRole` expected the minter revert, but the `vm.expectRevert` was declared after `vm.prank`, causing Foundry to capture the wrong caller in the revert payload and fail the equality check.
- **Debug steps:**
  1. Examined the failing revert message from `forge test` to see the unexpected account address.
  2. Reviewed the test sequence and noticed the expectation was registered after the prank setup, meaning the revert payload reflected the prank's caller rather than the intended account.
  3. Moved `vm.expectRevert` before `vm.prank` so the revert payload aligned with the expected unauthorized account.
- **Resolution:** Reordered the expectation and prank calls to ensure the revert compared against `STRANGER`.
- **Lesson learned:** When testing expected reverts in Foundry, register `vm.expectRevert` before altering the call context with `vm.prank` to keep the revert payload predictable.

### Challenge: UserRegistry setup failing due to missing prank scope
- **Source file:** `contracts/test/UserRegistry.t.sol`
- **Issue summary:** `setUp()` reverted with `AccessControlUnauthorizedAccount` because role grants were executed without impersonating the admin, so the default test contract address attempted the grants.
- **Debug steps:**
  1. Re-ran `forge test` to capture the failing revert selector and role hash.
  2. Checked the setup logic and confirmed the admin address was defined as a constant but never impersonated.
  3. Wrapped the initialization and role grants in `vm.startPrank(ADMIN)`/`vm.stopPrank()` to ensure the correct caller performed privileged actions.
- **Resolution:** Added prank scoping for the admin during registry initialization and role grants.
- **Lesson learned:** Always ensure Foundry tests impersonate privileged accounts when exercising role-restricted code, especially inside `setUp()`.

### Challenge: ComplianceManager test hitting `InvalidInitialization`
- **Source file:** `contracts/test/ComplianceManager.t.sol`
- **Issue summary:** Deploying `ComplianceManager` directly and calling `initialize` triggered `InvalidInitialization` because the contract disables initializers in its constructor.
- **Debug steps:**
  1. Observed the revert from the test suite pinpointing an initialization guard failure.
  2. Reviewed `ComplianceManager` and noted it uses UUPS with `_disableInitializers()` in the constructor, so the implementation must be invoked through a proxy.
  3. Updated the test to deploy an `ERC1967Proxy` pointing to the implementation with encoded initializer data.
- **Resolution:** Created a proxy instance in the test and interacted through it, matching real deployment behavior.
- **Lesson learned:** For UUPS contracts with disabled initializers, tests must simulate proxy deployments; direct initialization on the implementation contract will revert.

### Challenge: Post-initialization role grants failing for manager proxy
- **Source file:** `contracts/test/ComplianceManager.t.sol`
- **Issue summary:** Granting roles to the manager proxy still reverted because the admin prank ended before all role assignments were issued.
- **Debug steps:**
  1. After adopting the proxy, the test continued to fail with unauthorized errors on role grants.
  2. Checked the transaction order and realized the prank ended before calling `grantRole`, so the default test contract attempted the grant.
  3. Wrapped the registry role grant and manager role grant inside a single `vm.startPrank(ADMIN)` block.
- **Resolution:** Ensured the admin impersonation persisted through all privileged calls.
- **Lesson learned:** When chaining multiple privileged operations in tests, keep the prank scope active until all related calls complete.

## 2025-11-02 (later)

### Challenge: OpenZeppelin security module paths not found
- **Source file:** `contracts/src/MintEscrow.sol`
- **Issue summary:** `forge test` failed because OpenZeppelin v5 moves `ReentrancyGuard` and `Pausable` under `utils/`, but the contract imported them from the legacy `security/` path.
- **Debug steps:**
  1. Inspected the compiler error pointing to missing `security/ReentrancyGuard.sol`.
  2. Listed the vendored OpenZeppelin directory to confirm the new file layout.
  3. Updated the imports to `@openzeppelin/contracts/utils/ReentrancyGuard.sol` and `@openzeppelin/contracts/utils/Pausable.sol`.
- **Resolution:** Adjusted import paths to the correct module locations.
- **Lesson learned:** When vendoring OpenZeppelin v5+, double-check directory changes; many utilities moved from `security` to `utils`.

### Challenge: Granting MINTER_ROLE from wrong admin in MintEscrow tests
- **Source file:** `contracts/test/MintEscrow.t.sol`
- **Issue summary:** `setUp()` reverted because the test attempted to grant `MINTER_ROLE` to the escrow while impersonating `ADMIN`, but the CountryToken's default admin is the test contract itself.
- **Debug steps:**
  1. Ran the suite with `-vvvv` to capture the trace showing the revert during `CountryToken::grantRole`.
  2. Noticed the sender was the pranked `ADMIN`, which lacks the default admin role for `CountryToken`.
  3. Moved the grant call outside the prank scope so it executes as the deploying test contract.
- **Resolution:** Issued the `grantRole` call without `ADMIN` impersonation, allowing the default admin (test contract) to perform the action.
- **Lesson learned:** Pay attention to who owns role-based privileges after deployment; pranking as a different account can unintentionally strip required permissions.

### Challenge: Deployment script unable to write `deployments.json`
- **Source file:** `contracts/script/DeployFiatRails.s.sol`
- **Issue summary:** Running the Forge script on Anvil failed with `vm.writeFile` because Foundry disallowed writes to `deployments.json`.
- **Debug steps:**
  1. Inspected the stack trace to confirm the revert originated from `vm.writeFile`.
  2. Recalled Foundry’s filesystem sandbox and added `fs_permissions` to `contracts/foundry.toml` granting read-write access to the project root.
  3. Restarted Anvil for a clean fork and reran the script with the permission fix in place.
- **Resolution:** Updated Foundry configuration and re-executed the script, producing `deployments.json` successfully.
- **Lesson learned:** Forge scripts obey the same FS sandbox as tests; grant explicit `fs_permissions` before attempting to write artifacts like deployment manifests.
