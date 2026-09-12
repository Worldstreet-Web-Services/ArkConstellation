// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "forge-std/Test.sol";
import "account-abstraction/interfaces/PackedUserOperation.sol";
import "../src/MinimalEntryPoint.sol";
import "../src/SimplePaymaster.sol";

contract MinimalEntryPointTest is Test {
    MinimalEntryPoint entryPoint;
    SimplePaymaster paymaster;

    uint256 senderKey;
    address sender;

    function setUp() public {
        entryPoint = new MinimalEntryPoint();
        paymaster = new SimplePaymaster(IMinimalEntryPoint(address(entryPoint)));

        senderKey = 0xA11CE;
        sender = vm.addr(senderKey);

        vm.deal(address(this), 10 ether);
        paymaster.depositToEntryPoint{value: 5 ether}(5 ether);
    }

    function _packGasLimits(uint128 verificationGasLimit, uint128 callGasLimit) private pure returns (bytes32) {
        return bytes32((uint256(verificationGasLimit) << 128) | uint256(callGasLimit));
    }

    function _packGasFees(uint128 maxPriorityFeePerGas, uint128 maxFeePerGas) private pure returns (bytes32) {
        return bytes32((uint256(maxPriorityFeePerGas) << 128) | uint256(maxFeePerGas));
    }

    function _signedOp(
        uint256 nonce,
        bytes memory paymasterAndData,
        uint128 callGasLimit,
        uint128 verificationGasLimit,
        uint128 maxFeePerGas
    ) private view returns (PackedUserOperation memory op) {
        op = PackedUserOperation({
            sender: sender,
            nonce: nonce,
            initCode: "",
            callData: "",
            accountGasLimits: _packGasLimits(verificationGasLimit, callGasLimit),
            preVerificationGas: 21000,
            gasFees: _packGasFees(1 gwei, maxFeePerGas),
            paymasterAndData: paymasterAndData,
            signature: ""
        });

        bytes32 userOpHash = entryPoint.getUserOpHash(op);
        bytes32 ethSignedHash = keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", userOpHash));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(senderKey, ethSignedHash);
        op.signature = abi.encodePacked(r, s, v);
    }

    function test_SelfPaidOpSucceeds() public {
        PackedUserOperation memory op = _signedOp(0, "", 100_000, 100_000, 1 gwei);
        PackedUserOperation[] memory ops = new PackedUserOperation[](1);
        ops[0] = op;

        entryPoint.handleOps(ops, payable(address(this)));

        assertEq(entryPoint.nonce(sender), 1);
    }

    function test_PaymasterSponsoredOp_RefundsUnusedGas() public {
        bytes memory paymasterAndData = abi.encodePacked(address(paymaster));
        // Deliberately oversized limits vs. the trivial empty-callData call
        // actually performed, so the required-vs-actual gap is large.
        PackedUserOperation memory op = _signedOp(0, paymasterAndData, 1_000_000, 1_000_000, 1 gwei);
        PackedUserOperation[] memory ops = new PackedUserOperation[](1);
        ops[0] = op;

        uint256 depositBefore = entryPoint.balanceOf(address(paymaster));

        entryPoint.handleOps(ops, payable(address(this)));

        uint256 depositAfter = entryPoint.balanceOf(address(paymaster));
        uint256 charged = depositBefore - depositAfter;

        // Full gas-limit estimate would have been ~2,021,000 gas * 1 gwei.
        // Actual execution (a no-op call) uses far less; the paymaster
        // should be charged close to real usage, not the full estimate.
        assertLt(charged, 200_000 gwei, "paymaster overcharged - refund not applied");
        assertEq(entryPoint.nonce(sender), 1);
    }

    function test_FailingOpDoesNotRevertWholeBatch() public {
        // Independently-constructed ops: memory-to-memory struct assignment
        // in Solidity aliases rather than copies, so these must not share
        // an underlying struct or mutating one would mutate the other.
        PackedUserOperation memory badOp = _signedOp(5, "", 100_000, 100_000, 1 gwei); // wrong nonce for `sender`, which is still at 0
        PackedUserOperation memory goodOp = _signedOp(0, "", 100_000, 100_000, 1 gwei);

        PackedUserOperation[] memory ops = new PackedUserOperation[](2);
        ops[0] = badOp;
        ops[1] = goodOp;

        // Must not revert despite ops[0] failing.
        entryPoint.handleOps(ops, payable(address(this)));

        // ops[1] (the valid op) still went through.
        assertEq(entryPoint.nonce(sender), 1);
    }

    function test_InvalidSignatureRejected() public {
        PackedUserOperation memory op = _signedOp(0, "", 100_000, 100_000, 1 gwei);
        op.signature[0] = bytes1(uint8(op.signature[0]) ^ 0xFF); // corrupt the signature

        PackedUserOperation[] memory ops = new PackedUserOperation[](1);
        ops[0] = op;

        entryPoint.handleOps(ops, payable(address(this)));

        // Rejected: nonce must not have advanced.
        assertEq(entryPoint.nonce(sender), 0);
    }

    receive() external payable {}
}
