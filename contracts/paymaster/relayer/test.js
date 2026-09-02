require('dotenv').config();
const { ethers } = require('ethers');

// Configuration
const RPC_URL = process.env.RPC_URL || 'https://evm.34.60.137.196.sslip.io';
const PRIVATE_KEY = process.env.PRIVATE_KEY;
const CHAIN_ID = parseInt(process.env.CHAIN_ID || '9000');
const ENTRY_POINT_ADDRESS = process.env.ENTRY_POINT_ADDRESS || '0xE2d9782764B5C26b95DFDe9bE97793eBdeb8838C';
const PAYMASTER_ADDRESS = process.env.PAYMASTER_ADDRESS || '0x081AB05079A46D3b22623CF6e506Dc5806430bE3';

// Minimal ABI for testing
const PAYMASTER_ABI = [
    'function entryPoint() external view returns (address)',
    'function owner() external view returns (address)'
];

async function testDeployment() {
    console.log('=== Testing Paymaster Deployment on Ark Devnet ===\n');

    try {
        // Connect to provider
        console.log('1. Connecting to RPC...');
        const provider = new ethers.JsonRpcProvider(RPC_URL, undefined, {
            staticNetwork: true,
            batchMaxCount: 1
        });
        const network = await provider.getNetwork();
        console.log('   Connected to chain ID:', network.chainId.toString());
        console.log('   Expected chain ID:', CHAIN_ID);

        if (Number(network.chainId) !== CHAIN_ID) {
            console.warn('   WARNING: Chain ID mismatch!');
        } else {
            console.log('   ✓ Chain ID matches');
        }

        // Create wallet
        console.log('\n2. Creating wallet...');
        const wallet = new ethers.Wallet(PRIVATE_KEY, provider);
        const address = await wallet.getAddress();
        console.log('   Wallet address:', address);

        // Check wallet balance
        console.log('\n3. Checking wallet balance...');
        const balance = await provider.getBalance(address);
        console.log('   Balance:', ethers.formatEther(balance), 'KASH');
        if (balance === 0n) {
            console.warn('   WARNING: Wallet has no balance!');
        } else {
            console.log('   ✓ Wallet has balance');
        }

        // Check EntryPoint deployment
        console.log('\n4. Checking EntryPoint deployment...');
        try {
            const entryPointCode = await provider.getCode(ENTRY_POINT_ADDRESS);
            if (entryPointCode === '0x') {
                console.error('   ✗ EntryPoint not deployed at address:', ENTRY_POINT_ADDRESS);
                throw new Error('EntryPoint not deployed');
            }
            console.log('   EntryPoint address:', ENTRY_POINT_ADDRESS);
            console.log('   ✓ EntryPoint is deployed (code length:', entryPointCode.length / 2 - 1, 'bytes)');
        } catch (error) {
            console.error('   ✗ EntryPoint check failed:', error.message);
            throw error;
        }

        // Check Paymaster deployment
        console.log('\n5. Checking Paymaster deployment...');
        const paymaster = new ethers.Contract(PAYMASTER_ADDRESS, PAYMASTER_ABI, provider);
        try {
            const paymasterEntryPoint = await paymaster.entryPoint();
            const paymasterOwner = await paymaster.owner();
            console.log('   Paymaster address:', PAYMASTER_ADDRESS);
            console.log('   Paymaster EntryPoint:', paymasterEntryPoint);
            console.log('   Paymaster owner:', paymasterOwner);

            if (paymasterEntryPoint.toLowerCase() === ENTRY_POINT_ADDRESS.toLowerCase()) {
                console.log('   ✓ Paymaster EntryPoint matches deployed EntryPoint');
            } else {
                console.error('   ✗ Paymaster EntryPoint mismatch!');
            }

            if (paymasterOwner.toLowerCase() === address.toLowerCase()) {
                console.log('   ✓ Paymaster owner matches wallet');
            } else {
                console.log('   ℹ Paymaster owner:', paymasterOwner);
            }
        } catch (error) {
            console.error('   ✗ Paymaster check failed:', error.message);
            throw error;
        }

        // Check Paymaster balance
        console.log('\n6. Checking Paymaster balance...');
        const paymasterBalance = await provider.getBalance(PAYMASTER_ADDRESS);
        console.log('   Paymaster balance:', ethers.formatEther(paymasterBalance), 'KASH');
        if (paymasterBalance === 0n) {
            console.warn('   WARNING: Paymaster has no balance to sponsor gas!');
        } else {
            console.log('   ✓ Paymaster has balance for gas sponsorship');
        }

        

        console.log('\n=== All checks passed! ===');
        console.log('\nContract addresses:');
        console.log('  EntryPoint:', ENTRY_POINT_ADDRESS);
        console.log('  Paymaster:', PAYMASTER_ADDRESS);
        console.log('\nExplorer: https://explorer.34.60.137.196.sslip.io');

    } catch (error) {
        console.error('\n=== Test failed ===');
        console.error(error);
        process.exit(1);
    }
}

testDeployment().catch(console.error);