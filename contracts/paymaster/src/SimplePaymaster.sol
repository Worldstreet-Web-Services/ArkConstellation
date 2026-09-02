// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "account-abstraction/interfaces/IPaymaster.sol";
import "account-abstraction/interfaces/PackedUserOperation.sol";
import "./MinimalEntryPoint.sol";

/**
 * @title SimplePaymaster
 * @dev A simple paymaster that sponsors gas for all user operations.
 *      For MVP purposes, this paymaster does not enforce any restrictions.
 *      In production, you should add whitelisting, rate limiting, or other controls.
 */
contract SimplePaymaster is IPaymaster {
    // The EntryPoint contract
    IMinimalEntryPoint public immutable entryPoint;

    // Owner of the paymaster (can withdraw funds)
    address public owner;

    // Events
    event PaymasterSponsored(address indexed sender, uint256 gasUsed);
    event OwnerUpdated(address indexed oldOwner, address indexed newOwner);
    event FundsWithdrawn(address indexed to, uint256 amount);

    // Errors
    error Unauthorized();
    error InsufficientFunds();
    error InvalidSender();

    /**
     * @dev Constructor to initialize the paymaster with the EntryPoint address.
     * @param _entryPoint The address of the EntryPoint contract.
     */
    constructor(IMinimalEntryPoint _entryPoint) {
        entryPoint = _entryPoint;
        owner = msg.sender;
    }

    /**
     * @dev Validates the user operation and returns the context and validation result.
     *      For this simple paymaster, we sponsor all operations (return 0).
     * @param userOp The user operation to validate.
     * @param userOpHash The hash of the user operation.
     * @param maxCost The maximum cost that will be charged to the paymaster.
     * @return context The context data (empty for simple paymaster).
     * @return validationData The validation result (0 for success).
     */
    function validatePaymasterUserOp(
        PackedUserOperation calldata userOp,
        bytes32 userOpHash,
        uint256 maxCost
    ) external override returns (bytes memory context, uint256 validationData) {
        // Ensure the caller is the EntryPoint
        if (msg.sender != address(entryPoint)) {
            revert InvalidSender();
        }

        // For MVP, we sponsor all operations without restrictions
        // In production, add checks like:
        // - Whitelist sender addresses
        // - Rate limiting
        // - Gas cost limits
        emit PaymasterSponsored(userOp.sender, maxCost);
        return ("", 0);
    }

    /**
     * @dev Post-operation hook called after the user operation is executed.
     *      This is where we can perform any post-verification logic.
     * @param mode The post-operation mode.
     * @param context The context data from validation.
     * @param actualGasCost The actual gas cost of the operation.
     * @param actualUserOpFeePerGas The actual fee per gas paid by the user operation.
     */
    function postOp(
        PostOpMode mode,
        bytes calldata context,
        uint256 actualGasCost,
        uint256 actualUserOpFeePerGas
    ) external override {
        // Ensure the caller is the EntryPoint
        if (msg.sender != address(entryPoint)) {
            revert InvalidSender();
        }

        // No post-op logic needed for simple paymaster
    }

    /**
     * @dev Allows the owner to withdraw funds from the paymaster.
     * @param to The address to send funds to.
     * @param amount The amount to withdraw.
     */
    function withdrawFunds(address payable to, uint256 amount) external {
        if (msg.sender != owner) {
            revert Unauthorized();
        }
        if (address(this).balance < amount) {
            revert InsufficientFunds();
        }
        to.transfer(amount);
        emit FundsWithdrawn(to, amount);
    }

    /**
     * @dev Allows the owner to transfer ownership to a new address.
     * @param newOwner The new owner address.
     */
    function transferOwnership(address newOwner) external {
        if (msg.sender != owner) {
            revert Unauthorized();
        }
        emit OwnerUpdated(owner, newOwner);
        owner = newOwner;
    }

    

    /**
     * @dev Allows the paymaster to receive ETH for sponsoring gas.
     */
    receive() external payable {}

    /**
     * @dev Deposit funds into the EntryPoint for this paymaster.
     * @param amount The amount to deposit.
     */
    function depositToEntryPoint(uint256 amount) external payable {
        if (msg.sender != owner) {
            revert Unauthorized();
        }
        entryPoint.depositTo{value: amount}(address(this));
    }

    /**
     * @dev Withdraw funds from the EntryPoint for this paymaster.
     * @param to The address to send funds to.
     * @param amount The amount to withdraw.
     */
    function withdrawFromEntryPoint(address payable to, uint256 amount) external {
        if (msg.sender != owner) {
            revert Unauthorized();
        }
        entryPoint.withdrawTo(to, amount);
    }
}