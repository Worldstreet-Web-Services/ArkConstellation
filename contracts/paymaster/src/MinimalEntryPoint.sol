// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "account-abstraction/interfaces/PackedUserOperation.sol";

/**
 * @dev Minimal IEntryPoint interface for MVP
 */
interface IMinimalEntryPoint {
    function depositTo(address account) external payable;
    function withdrawTo(address payable withdrawAddress, uint256 withdrawAmount) external;
}

/**
 * @dev Minimal IPaymaster interface
 */
interface IMinimalPaymaster {
    function validatePaymasterUserOp(
        PackedUserOperation calldata userOp,
        bytes32 userOpHash,
        uint256 maxCost
    ) external returns (bytes memory context, uint256 validationData);
}

/**
 * @title MinimalEntryPoint
 * @dev A minimal implementation of ERC-4337 EntryPoint for MVP purposes.
 *      This includes only the essential functions needed for Paymaster sponsorship.
 *      For production, use the full EntryPoint implementation from eth-infinitism.
 */
contract MinimalEntryPoint is IMinimalEntryPoint {
    // Errors
    error InvalidSignature();
    error FailedOp(uint256 index, address sender, string reason);
    error NotPaymaster();
    error InsufficientDeposit();

    // Events
    event UserOperationEvent(
        bytes32 indexed userOpHash,
        address indexed sender,
        address indexed paymaster,
        bool success,
        uint256 actualGasCost,
        uint256 actualGasUsed
    );

    event DepositChanged(address indexed account, uint256 newBalance);

    // Storage
    mapping(address => uint256) public balanceOf;
    mapping(address => uint256) public nonce;

    // Constants
    uint256 private constant VALID_SIG = 0;
    uint256 private constant SIG_VALIDATION_FAILED = 1;

    /**
     * @dev Execute a batch of user operations
     * @param ops Array of user operations to execute
     * @param beneficiary Address to receive the gas refund
     */
    function handleOps(
        PackedUserOperation[] calldata ops,
        address payable beneficiary
    ) external payable {
        uint256 opslen = ops.length;
        uint256 collected = 0;
        for (uint256 i = 0; i < opslen; i++) {
            collected += _handleOp(ops[i], beneficiary);
        }

        // Send only what was actually collected from this batch's paymasters
        // to beneficiary - never the whole contract balance, which would
        // include every other paymaster's untouched deposit.
        if (beneficiary != address(0) && collected > 0) {
            uint256 payout = collected > address(this).balance ? address(this).balance : collected;
            beneficiary.transfer(payout);
        }
    }

    /**
     * @dev Internal function to handle a single user operation
     * @return gasCost the amount deducted from the sponsoring paymaster's deposit, if any
     */
    function _handleOp(
        PackedUserOperation calldata op,
        address payable beneficiary
    ) private returns (uint256 gasCost) {
        address sender = op.sender;

        // Check nonce
        if (nonce[sender] != op.nonce) {
            revert FailedOp(0, sender, "Invalid nonce");
        }

        bytes32 userOpHash = getUserOpHash(op);
        _validateSignature(sender, userOpHash, op.signature);

        // Parse paymaster address
        address paymaster = _parsePaymasterAndData(op.paymasterAndData);
        uint256 requiredGas = _getRequiredGas(op);

        // Deduct from paymaster deposit before the external validation call
        // so a reentrant call from the paymaster sees the debited balance.
        if (paymaster != address(0)) {
            if (balanceOf[paymaster] < requiredGas) {
                revert InsufficientDeposit();
            }
            balanceOf[paymaster] -= requiredGas;
            gasCost = requiredGas;
        }

        // Increment nonce before any external call for the same reason.
        nonce[sender]++;

        // Validate with paymaster if present
        if (paymaster != address(0)) {
            (, uint256 validationData) = IMinimalPaymaster(paymaster).validatePaymasterUserOp(
                op,
                userOpHash,
                requiredGas
            );

            if (validationData != VALID_SIG) {
                revert FailedOp(0, sender, "Paymaster validation failed");
            }
        }

        // Execute the call
        (bool success, bytes memory result) = sender.call{gas: _getCallGasLimit(op)}(op.callData);

        if (!success) {
            revert FailedOp(0, sender, _getRevertMessage(result));
        }

        emit UserOperationEvent(
            userOpHash,
            sender,
            paymaster,
            success,
            gasCost,
            0 // actualGasUsed - simplified for MVP
        );
    }

    /**
     * @dev Recover the signer of userOpHash from op.signature and require it to
     *      match sender. This minimal EntryPoint has no account contracts to call
     *      into for validation (no IAccount implementations exist in this repo),
     *      so sender is treated as the EOA that must have signed the operation.
     */
    function _validateSignature(
        address sender,
        bytes32 userOpHash,
        bytes calldata signature
    ) private pure {
        if (signature.length != 65) {
            revert InvalidSignature();
        }

        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 32))
            v := byte(0, calldataload(add(signature.offset, 64)))
        }

        // Reject upper-range s to avoid ECDSA signature malleability.
        if (uint256(s) > 0x7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5D576E7357A4501DDFE92F46681B20A0) {
            revert InvalidSignature();
        }
        if (v != 27 && v != 28) {
            revert InvalidSignature();
        }

        bytes32 ethSignedHash = keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", userOpHash));
        address recovered = ecrecover(ethSignedHash, v, r, s);
        if (recovered == address(0) || recovered != sender) {
            revert InvalidSignature();
        }
    }

    /**
     * @dev Get the hash of a user operation
     */
    function getUserOpHash(PackedUserOperation calldata userOp) public view returns (bytes32) {
        return keccak256(abi.encode(userOp, address(this), block.chainid));
    }

    /**
     * @dev Deposit funds for an account
     */
    function depositTo(address account) external payable {
        balanceOf[account] += msg.value;
        emit DepositChanged(account, balanceOf[account]);
    }

    /**
     * @dev Withdraw funds from the caller's own deposit to an arbitrary recipient.
     *      msg.sender is the depositor being debited; withdrawAddress is just the
     *      recipient, and the two may differ (e.g. a paymaster contract withdrawing
     *      to its owner's EOA).
     */
    function withdrawTo(address payable withdrawAddress, uint256 withdrawAmount) external {
        if (balanceOf[msg.sender] < withdrawAmount) {
            revert InsufficientDeposit();
        }
        balanceOf[msg.sender] -= withdrawAmount;
        withdrawAddress.transfer(withdrawAmount);
    }

    /**
     * @dev Get the current nonce for an account
     */
    function getNonce(address sender, uint192 key) external view returns (uint256) {
        // Simplified: ignore key for MVP
        return nonce[sender];
    }

    /**
     * @dev Parse paymaster address from paymasterAndData
     */
    function _parsePaymasterAndData(bytes calldata paymasterAndData) private pure returns (address) {
        if (paymasterAndData.length < 20) {
            return address(0);
        }
        return address(bytes20(paymasterAndData[0:20]));
    }

    /**
     * @dev Calculate required gas for a user operation
     */
    function _getRequiredGas(PackedUserOperation calldata op) private pure returns (uint256) {
        uint128 callGasLimit = uint128(uint256(op.accountGasLimits));
        uint128 verificationGasLimit = uint128(uint256(op.accountGasLimits) >> 128);
        return uint256(callGasLimit) + uint256(verificationGasLimit) + op.preVerificationGas;
    }

    /**
     * @dev Extract call gas limit from accountGasLimits
     */
    function _getCallGasLimit(PackedUserOperation calldata op) private pure returns (uint256) {
        return uint256(uint128(uint256(op.accountGasLimits)));
    }

    /**
     * @dev Get revert message from result bytes
     */
    function _getRevertMessage(bytes memory result) private pure returns (string memory) {
        if (result.length < 68) {
            return "Transaction reverted";
        }
        // Skip the error selector (4 bytes)
        bytes memory revertData = new bytes(result.length - 4);
        for (uint256 i = 0; i < revertData.length; i++) {
            revertData[i] = result[i + 4];
        }
        return abi.decode(revertData, (string));
    }

    /**
     * @dev Receive ETH
     */
    receive() external payable {}
}

