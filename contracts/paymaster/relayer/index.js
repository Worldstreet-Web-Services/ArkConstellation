require('dotenv').config();
const { ethers } = require('ethers');

// Configuration
const RPC_URL = process.env.RPC_URL || 'https://evm.34.60.137.196.sslip.io';
const PRIVATE_KEY = process.env.RELAYER_PRIVATE_KEY || process.env.PRIVATE_KEY;
const CHAIN_ID = parseInt(process.env.CHAIN_ID || '9000');
const ENTRY_POINT_ADDRESS = process.env.ENTRY_POINT_ADDRESS || '0xE2d9782764B5C26b95DFDe9bE97793eBdeb8838C';
const PAYMASTER_ADDRESS = process.env.PAYMASTER_ADDRESS || '0x081AB05079A46D3b22623CF6e506Dc5806430bE3';

// ABI for EntryPoint handleOps function
const ENTRY_POINT_ABI = [
    'function handleOps((address sender, uint256 nonce, bytes initCode, bytes callData, uint256 callGasLimit, uint256 verificationGasLimit, uint256 preVerificationGas, uint256 maxFeePerGas, uint256 maxPriorityFeePerGas, bytes paymasterAndData, bytes signature)[] calldata ops, address beneficiary) external payable',
    'function getUserOpHash((address sender, uint256 nonce, bytes initCode, bytes callData, uint256 callGasLimit, uint256 verificationGasLimit, uint256 preVerificationGas, uint256 maxFeePerGas, uint256 maxPriorityFeePerGas, bytes paymasterAndData, bytes signature) calldata userOp) external view returns (bytes32)',
    'function getNonce(address sender, uint192 key) external view returns (uint256 nonce)'
];

// Simple relayer class
class PaymasterRelayer {
    constructor() {
        if (!PRIVATE_KEY) {
            throw new Error('RELAYER_PRIVATE_KEY or PRIVATE_KEY must be set in .env');
        }

        this.provider = new ethers.JsonRpcProvider(RPC_URL);
        this.wallet = new ethers.Wallet(PRIVATE_KEY, this.provider);
        this.entryPoint = new ethers.Contract(ENTRY_POINT_ADDRESS, ENTRY_POINT_ABI, this.wallet);
        this.paymasterAddress = PAYMASTER_ADDRESS;
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

            // Execute the user operation
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

    // Example: Sponsor a user operation (this would be called with actual user ops)
    // const exampleUserOp = {
    //     sender: '0x...',
    //     nonce: 0n,
    //     initCode: '0x',
    //     callData: '0x...',
    //     callGasLimit: 100000n,
    //     verificationGasLimit: 100000n,
    //     preVerificationGas: 21000n,
    //     maxFeePerGas: 1000000000n,
    //     maxPriorityFeePerGas: 1000000000n,
    //     paymasterAndData: PAYMASTER_ADDRESS + '00'.repeat(20), // Paymaster address + empty data
    //     signature: '0x...'
    // };
    // await relayer.sponsorUserOp(exampleUserOp);
}

// Export for use in other scripts
module.exports = PaymasterRelayer;

// Run if executed directly
if (require.main === module) {
    main().catch(console.error);
}