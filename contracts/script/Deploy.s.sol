// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Script} from "forge-std/Script.sol";
import {console2} from "forge-std/console2.sol";
import {QuantumPayments} from "../src/QuantumPayments.sol";
import {IERC20Metadata} from "@openzeppelin/contracts/token/ERC20/extensions/IERC20Metadata.sol";

contract Deploy is Script {
    error WrongChain(uint256 actual);
    error InvalidToken(address token);
    error WrongDecimals(uint8 actual);

    address private constant TUSD8 = 0x456569838f1d43009A91ebA4263147Afd7233483;

    function run() external {
        if (block.chainid != 44005) revert WrongChain(block.chainid);
        address token = vm.envOr("TUSD8_ADDRESS", TUSD8);
        if (token.code.length == 0) revert InvalidToken(token);
        uint8 decimals = IERC20Metadata(token).decimals();
        if (decimals != 6) revert WrongDecimals(decimals);
        vm.startBroadcast();
        QuantumPayments payments = new QuantumPayments(token);
        vm.stopBroadcast();
        console2.log("TOKEN", token);
        console2.log("QUANTUM_PAYMENTS", address(payments));
    }
}
