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
    error InvalidCaller();

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
        for (uint256 i = 0; i < opslen; i++) {
            _handleOp(ops[i], beneficiary);
        }

        // Send remaining gas to beneficiary
        if (beneficiary != address(0) && address(this).balance > 0) {
            beneficiary.transfer(address(this).balance);
        }
    }

    /**
     * @dev Internal function to handle a single user operation
     */
    function _handleOp(
        PackedUserOperation calldata op,
        address payable beneficiary
    ) private {
        address sender = op.sender;

        // Check nonce
        if (nonce[sender] != op.nonce) {
            revert FailedOp(0, sender, "Invalid nonce");
        }

        // Parse paymaster address
        address paymaster = _parsePaymasterAndData(op.paymasterAndData);

        // Validate with paymaster if present
        if (paymaster != address(0)) {
            bytes32 userOpHash = getUserOpHash(op);
            (, uint256 validationData) = IMinimalPaymaster(paymaster).validatePaymasterUserOp(
                op,
                userOpHash,
                _getRequiredGas(op)
            );

            if (validationData != VALID_SIG) {
                revert FailedOp(0, sender, "Paymaster validation failed");
            }

            // Deduct from paymaster deposit
            uint256 requiredGas = _getRequiredGas(op);
            if (balanceOf[paymaster] < requiredGas) {
                revert InsufficientDeposit();
            }
            balanceOf[paymaster] -= requiredGas;
        }

        // Increment nonce
        nonce[sender]++;

        // Execute the call
        bool success;
        bytes memory result;
        (success, result) = sender.call{gas: _getCallGasLimit(op)}(op.callData);

        if (!success) {
            revert FailedOp(0, sender, _getRevertMessage(result));
        }

        emit UserOperationEvent(
            getUserOpHash(op),
            sender,
            paymaster,
            success,
            0, // actualGasCost - simplified for MVP
            0  // actualGasUsed - simplified for MVP
        );
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
     * @dev Withdraw funds to an account
     */
    function withdrawTo(address payable withdrawAddress, uint256 withdrawAmount) external {
        if (msg.sender != withdrawAddress) {
            revert InvalidCaller();
        }
        if (balanceOf[withdrawAddress] < withdrawAmount) {
            revert InsufficientDeposit();
        }
        balanceOf[withdrawAddress] -= withdrawAmount;
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

