// SPDX-License-Identifier: MIT
pragma solidity 0.8.33;

import {Script} from "forge-std/Script.sol";

import {AegisEscrow} from "../src/AegisEscrow.sol";

contract DeployAegisEscrow is Script {
    function run() external returns (AegisEscrow deployed) {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address admin = vm.envAddress("AEGIS_ESCROW_ADMIN");

        vm.startBroadcast(deployerPrivateKey);
        deployed = new AegisEscrow(admin);
        vm.stopBroadcast();
    }
}

