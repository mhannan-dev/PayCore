// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Script} from "forge-std/Script.sol";
import {console2} from "forge-std/console2.sol";
import {QuantumPayments} from "../src/QuantumPayments.sol";

contract Deploy is Script {
    address private constant TUSD8 = 0x456569838f1d43009A91ebA4263147Afd7233483;

    function run() external {
        address token = vm.envOr("TUSD8_ADDRESS", TUSD8);
        vm.startBroadcast();
        QuantumPayments payments = new QuantumPayments(token);
        vm.stopBroadcast();
        console2.log("TOKEN", token);
        console2.log("QUANTUM_PAYMENTS", address(payments));
    }
}
