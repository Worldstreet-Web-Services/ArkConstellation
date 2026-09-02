// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "forge-std/Script.sol";
import "../src/SimplePaymaster.sol";
import "../src/MinimalEntryPoint.sol";

contract DeployScript is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        uint256 chainId = vm.envUint("CHAIN_ID");

        vm.startBroadcast(deployerPrivateKey);

        // Deploy EntryPoint contract
        MinimalEntryPoint entryPoint = new MinimalEntryPoint();
        console.log("EntryPoint deployed to:", address(entryPoint));

        // Deploy SimplePaymaster contract
        SimplePaymaster paymaster = new SimplePaymaster(entryPoint);
        console.log("SimplePaymaster deployed to:", address(paymaster));

        // Fund the paymaster with some ETH for gas sponsorship
        uint256 fundAmount = 1 ether; // 1 KASH for gas sponsorship
        (bool success, ) = address(paymaster).call{value: fundAmount}("");
        require(success, "Failed to fund paymaster");
        console.log("Paymaster funded with:", fundAmount, "wei");

        // Deposit funds to EntryPoint for the paymaster
        uint256 depositAmount = 0.5 ether; // 0.5 KASH for EntryPoint deposit
        paymaster.depositToEntryPoint{value: depositAmount}(depositAmount);
        console.log("Paymaster deposited", depositAmount, "wei to EntryPoint");

        vm.stopBroadcast();

        console.log("Deployment completed on chain ID:", chainId);
        console.log("EntryPoint:", address(entryPoint));
        console.log("SimplePaymaster:", address(paymaster));
    }
}