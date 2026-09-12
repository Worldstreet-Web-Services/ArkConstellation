// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "forge-std/Test.sol";
import "account-abstraction/core/EntryPoint.sol";
import "account-abstraction/accounts/SimpleAccountFactory.sol";
import "account-abstraction/accounts/SimpleAccount.sol";
import "account-abstraction/interfaces/PackedUserOperation.sol";
import "../src/SimplePaymaster.sol";

/// Integration test against the real, audited eth-infinitism EntryPoint -
/// replaces the old MinimalEntryPoint.t.sol, which tested a custom
/// reimplementation that this PR removed (see issue #44).
contract PaymasterIntegrationTest is Test {
    EntryPoint entryPoint;
    SimpleAccountFactory accountFactory;
    SimplePaymaster paymaster;

    uint256 ownerKey;
    address owner;

    function setUp() public {
        entryPoint = new EntryPoint();
        accountFactory = new SimpleAccountFactory(entryPoint);
        paymaster = new SimplePaymaster(entryPoint);

        ownerKey = 0xA11CE;
        owner = vm.addr(ownerKey);

        vm.deal(address(this), 10 ether);
        paymaster.depositToEntryPoint{value: 5 ether}(5 ether);
    }

    function _packGasLimits(uint128 verificationGasLimit, uint128 callGasLimit) private pure returns (bytes32) {
        return bytes32((uint256(verificationGasLimit) << 128) | uint256(callGasLimit));
    }

    function _packGasFees(uint128 maxPriorityFeePerGas, uint128 maxFeePerGas) private pure returns (bytes32) {
        return bytes32((uint256(maxPriorityFeePerGas) << 128) | uint256(maxFeePerGas));
    }

    /// Real EntryPoint's paymasterAndData layout: paymaster (20 bytes) ||
    /// paymasterVerificationGasLimit (16 bytes) || paymasterPostOpGasLimit (16 bytes).
    function _packPaymasterAndData(address pm, uint128 verificationGasLimit, uint128 postOpGasLimit)
        private
        pure
        returns (bytes memory)
    {
        return abi.encodePacked(pm, verificationGasLimit, postOpGasLimit);
    }

    function test_NewAccount_SponsoredOp_DeploysAndExecutes() public {
        uint256 salt = 0;
        address sender = accountFactory.getAddress(owner, salt);
        assertEq(sender.code.length, 0, "account should not be deployed yet");

        // Fund the not-yet-deployed account's own EntryPoint deposit isn't
        // needed since the paymaster sponsors this op.
        bytes memory initCode = abi.encodePacked(
            address(accountFactory), abi.encodeCall(SimpleAccountFactory.createAccount, (owner, salt))
        );

        PackedUserOperation memory op = PackedUserOperation({
            sender: sender,
            nonce: 0,
            initCode: initCode,
            callData: "",
            accountGasLimits: _packGasLimits(500_000, 200_000),
            preVerificationGas: 50_000,
            gasFees: _packGasFees(1 gwei, 1 gwei),
            paymasterAndData: _packPaymasterAndData(address(paymaster), 100_000, 50_000),
            signature: ""
        });

        bytes32 userOpHash = entryPoint.getUserOpHash(op);
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(ownerKey, userOpHash);
        op.signature = abi.encodePacked(r, s, v);

        PackedUserOperation[] memory ops = new PackedUserOperation[](1);
        ops[0] = op;

        uint256 paymasterDepositBefore = entryPoint.balanceOf(address(paymaster));

        // The real EntryPoint's reentrancy guard requires the direct caller
        // to be an EOA (tx.origin == msg.sender, no code) - a contract like
        // this test can't call handleOps directly without pranking as one.
        address bundler = makeAddr("bundler");
        vm.prank(bundler, bundler);
        entryPoint.handleOps(ops, payable(address(this)));

        assertGt(sender.code.length, 0, "account should be deployed by initCode");
        assertEq(SimpleAccount(payable(sender)).owner(), owner);
        assertLt(
            entryPoint.balanceOf(address(paymaster)), paymasterDepositBefore, "paymaster deposit should be charged"
        );
    }

    function test_ExistingAccount_SelfPaidOp_NoPaymaster() public {
        uint256 salt = 1;
        address sender = accountFactory.getAddress(owner, salt);

        // Pre-deploy the account directly (bypassing initCode/senderCreator)
        // and fund its own EntryPoint deposit, to exercise the self-paying
        // path independent of the paymaster.
        vm.prank(address(entryPoint.senderCreator()));
        accountFactory.createAccount(owner, salt);
        vm.deal(address(this), 10 ether);
        entryPoint.depositTo{value: 2 ether}(sender);

        PackedUserOperation memory op = PackedUserOperation({
            sender: sender,
            nonce: 0,
            initCode: "",
            callData: "",
            accountGasLimits: _packGasLimits(500_000, 200_000),
            preVerificationGas: 50_000,
            gasFees: _packGasFees(1 gwei, 1 gwei),
            paymasterAndData: "",
            signature: ""
        });

        bytes32 userOpHash = entryPoint.getUserOpHash(op);
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(ownerKey, userOpHash);
        op.signature = abi.encodePacked(r, s, v);

        PackedUserOperation[] memory ops = new PackedUserOperation[](1);
        ops[0] = op;

        uint256 depositBefore = entryPoint.balanceOf(sender);

        address bundler = makeAddr("bundler2");
        vm.prank(bundler, bundler);
        entryPoint.handleOps(ops, payable(address(this)));

        assertLt(entryPoint.balanceOf(sender), depositBefore, "sender's own deposit should be charged");
    }

    receive() external payable {}
}
