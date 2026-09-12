require('dotenv').config();
const { ethers } = require('ethers');

// Configuration
const RPC_URL = process.env.RPC_URL || 'https://evm.34.60.137.196.sslip.io';
const PRIVATE_KEY = process.env.RELAYER_PRIVATE_KEY || process.env.PRIVATE_KEY;
const CHAIN_ID = parseInt(process.env.CHAIN_ID || '9000');
// No hardcoded fallback addresses here: this relayer now targets the real
// eth-infinitism EntryPoint (see issue #44), which has not been deployed to
// any live network yet. Defaulting to the old MinimalEntryPoint deployment's
// address would silently point at a completely different, now-removed
// contract - fail loudly instead until real addresses are configured.
const ENTRY_POINT_ADDRESS = process.env.ENTRY_POINT_ADDRESS;
const PAYMASTER_ADDRESS = process.env.PAYMASTER_ADDRESS;
const SIMPLE_ACCOUNT_FACTORY_ADDRESS = process.env.SIMPLE_ACCOUNT_FACTORY_ADDRESS;

// ABI for the real EntryPoint (PackedUserOperation shape is unchanged from
// the old MinimalEntryPoint - accountGasLimits/gasFees packed into two
// bytes32 fields - but handleOps is not payable, and paymasterAndData now
// has a structured layout, see buildPaymasterAndData below).
const ENTRY_POINT_ABI = [
    'function handleOps(tuple(address sender, uint256 nonce, bytes initCode, bytes callData, bytes32 accountGasLimits, uint256 preVerificationGas, bytes32 gasFees, bytes paymasterAndData, bytes signature)[] calldata ops, address payable beneficiary) external',
    'function getUserOpHash(tuple(address sender, uint256 nonce, bytes initCode, bytes callData, bytes32 accountGasLimits, uint256 preVerificationGas, bytes32 gasFees, bytes paymasterAndData, bytes signature) calldata userOp) external view returns (bytes32)',
    'function getNonce(address sender, uint192 key) external view returns (uint256 nonce)',
    'function balanceOf(address account) external view returns (uint256)'
];

// ABI for SimpleAccountFactory - lets the relayer predict a not-yet-deployed
// account's address and build the initCode to deploy it on first use.
const SIMPLE_ACCOUNT_FACTORY_ABI = [
    'function getAddress(address owner, uint256 salt) external view returns (address)',
    'function createAccount(address owner, uint256 salt) external returns (address)'
];

// Standard packed layout for a PackedUserOperation's paymasterAndData field:
// paymaster address (20 bytes) || paymasterVerificationGasLimit (16 bytes) ||
// paymasterPostOpGasLimit (16 bytes) || paymasterData (optional, unused here).
function buildPaymasterAndData(paymasterAddress, verificationGasLimit, postOpGasLimit) {
    return ethers.concat([
        paymasterAddress,
        ethers.zeroPadValue(ethers.toBeHex(verificationGasLimit), 16),
        ethers.zeroPadValue(ethers.toBeHex(postOpGasLimit), 16)
    ]);
}

// Pack two 128-bit values into a single bytes32 (used for both
// accountGasLimits = verificationGasLimit||callGasLimit and
// gasFees = maxPriorityFeePerGas||maxFeePerGas).
function packUint128Pair(high, low) {
    return ethers.zeroPadValue(
        ethers.toBeHex((BigInt(high) << 128n) | BigInt(low)),
        32
    );
}

// Simple relayer class
class PaymasterRelayer {
    constructor() {
        if (!PRIVATE_KEY) {
            throw new Error('RELAYER_PRIVATE_KEY or PRIVATE_KEY must be set in .env');
        }
        if (!ENTRY_POINT_ADDRESS || !PAYMASTER_ADDRESS) {
            throw new Error('ENTRY_POINT_ADDRESS and PAYMASTER_ADDRESS must be set in .env (no default - see comment above)');
        }

        this.provider = new ethers.JsonRpcProvider(RPC_URL);
        this.wallet = new ethers.Wallet(PRIVATE_KEY, this.provider);
        this.entryPoint = new ethers.Contract(ENTRY_POINT_ADDRESS, ENTRY_POINT_ABI, this.wallet);
        this.paymasterAddress = PAYMASTER_ADDRESS;

        if (SIMPLE_ACCOUNT_FACTORY_ADDRESS) {
            this.accountFactory = new ethers.Contract(SIMPLE_ACCOUNT_FACTORY_ADDRESS, SIMPLE_ACCOUNT_FACTORY_ABI, this.provider);
        }
    }

    /**
     * Predict a SimpleAccount's counterfactual address for an owner, and the
     * initCode to deploy it (if it isn't already deployed).
     * @param {string} owner - The EOA that will control the account.
     * @param {bigint} salt - Salt for the CREATE2 deployment (default 0).
     * @returns {Promise<{sender: string, initCode: string, deployed: boolean}>}
     */
    async getOrPredictAccount(owner, salt = 0n) {
        if (!this.accountFactory) {
            throw new Error('SIMPLE_ACCOUNT_FACTORY_ADDRESS must be set in .env to use account provisioning');
        }

        const sender = await this.accountFactory.getAddress(owner, salt);
        const code = await this.provider.getCode(sender);
        const deployed = code !== '0x';

        const initCode = deployed
            ? '0x'
            : ethers.concat([
                SIMPLE_ACCOUNT_FACTORY_ADDRESS,
                this.accountFactory.interface.encodeFunctionData('createAccount', [owner, salt])
            ]);

        return { sender, initCode, deployed };
    }

    /**
     * Sponsor a user operation by calling handleOps on the EntryPoint
     * @param {Object} userOp - The user operation to sponsor
     * @param {string} beneficiary - The address to receive the gas refund (usually the relayer)
     */
    async sponsorUserOp(userOp, beneficiary = null) {
        try {
            console.log('Sponsoring user operation...');
            console.log('UserOp sender:', userOp.sender);
            console.log('UserOp nonce:', userOp.nonce.toString());

            // Set beneficiary to relayer if not provided
            const actualBeneficiary = beneficiary || await this.wallet.getAddress();

            // Calculate the userOp hash for logging
            const userOpHash = await this.entryPoint.getUserOpHash(userOp);
            console.log('UserOp hash:', userOpHash);

            // Estimate gas
            const gasEstimate = await this.entryPoint.handleOps.estimateGas([userOp], actualBeneficiary);
            console.log('Estimated gas:', gasEstimate.toString());

            // Get current gas price
            const feeData = await this.provider.getFeeData();
            console.log('Gas price:', feeData.gasPrice?.toString());

            // Execute the user operation. handleOps is not payable on the
            // real EntryPoint, so no value is attached here.
            const tx = await this.entryPoint.handleOps([userOp], actualBeneficiary, {
                gasLimit: gasEstimate * 2n, // Add buffer
                maxFeePerGas: feeData.maxFeePerGas,
                maxPriorityFeePerGas: feeData.maxPriorityFeePerGas
            });

            console.log('Transaction sent:', tx.hash);
            console.log('Waiting for confirmation...');

            const receipt = await tx.wait();
            console.log('Transaction confirmed:', receipt.hash);
            console.log('Gas used:', receipt.gasUsed.toString());

            return receipt;
        } catch (error) {
            console.error('Error sponsoring user operation:', error);
            throw error;
        }
    }

    /**
     * Get the current nonce for a sender address
     * @param {string} sender - The sender address
     * @param {number} key - The nonce key (default 0)
     */
    async getNonce(sender, key = 0) {
        try {
            const nonce = await this.entryPoint.getNonce(sender, key);
            console.log(`Nonce for ${sender}:`, nonce.toString());
            return nonce;
        } catch (error) {
            console.error('Error getting nonce:', error);
            throw error;
        }
    }

    /**
     * Check the relayer's balance
     */
    async getBalance() {
        try {
            const balance = await this.provider.getBalance(await this.wallet.getAddress());
            console.log('Relayer balance:', ethers.formatEther(balance), 'KASH');
            return balance;
        } catch (error) {
            console.error('Error getting balance:', error);
            throw error;
        }
    }

    /**
     * Start a simple polling loop to check for pending user operations
     * This is a basic implementation - in production, you'd use event listeners or a message queue
     */
    async startPolling(intervalMs = 5000) {
        console.log('Starting relayer polling...');
        console.log('Relayer address:', await this.wallet.getAddress());
        console.log('Chain ID:', CHAIN_ID);
        console.log('EntryPoint:', ENTRY_POINT_ADDRESS);
        console.log('Paymaster:', this.paymasterAddress);

        await this.getBalance();

        // In a real implementation, you would:
        // 1. Listen for UserOperation events from the EntryPoint
        // 2. Query a database or message queue for pending operations
        // 3. Implement proper error handling and retry logic

        console.log('Polling started (this is a basic implementation)');
        console.log('To sponsor a specific user operation, use the sponsorUserOp method directly');
    }
}

// Example usage
async function main() {
    const relayer = new PaymasterRelayer();

    // Start polling
    await relayer.startPolling();

    // Example: sponsor a user operation for a not-yet-deployed SimpleAccount.
    // The account gets deployed as part of this same handleOps call via initCode.
    // const ownerWallet = new ethers.Wallet(process.env.USER_PRIVATE_KEY);
    // const { sender, initCode } = await relayer.getOrPredictAccount(ownerWallet.address, 0n);
    //
    // const userOp = {
    //     sender,
    //     nonce: await relayer.getNonce(sender),
    //     initCode,
    //     callData: '0x...',
    //     accountGasLimits: packUint128Pair(500_000n, 200_000n), // verificationGasLimit || callGasLimit
    //     preVerificationGas: 50_000n,
    //     gasFees: packUint128Pair(1_000_000_000n, 1_000_000_000n), // maxPriorityFeePerGas || maxFeePerGas
    //     paymasterAndData: buildPaymasterAndData(relayer.paymasterAddress, 100_000n, 50_000n),
    //     signature: '0x'
    // };
    // const userOpHash = await relayer.entryPoint.getUserOpHash(userOp);
    // userOp.signature = ownerWallet.signingKey.sign(userOpHash).serialized; // raw digest, no eth_sign prefix
    // await relayer.sponsorUserOp(userOp);
}

// Export for use in other scripts
module.exports = PaymasterRelayer;
module.exports.buildPaymasterAndData = buildPaymasterAndData;
module.exports.packUint128Pair = packUint128Pair;

// Run if executed directly
if (require.main === module) {
    main().catch(console.error);
}
