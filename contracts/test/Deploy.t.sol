// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {Deploy} from "../script/Deploy.s.sol";

contract DeployTest is Test {
    address private constant TOKEN = 0x456569838f1d43009A91ebA4263147Afd7233483;
    Deploy private deployment;

    function setUp() public {
        deployment = new Deploy();
        vm.setEnv("TUSD8_ADDRESS", vm.toString(TOKEN));
    }

    function test_rejectsWrongChainBeforeBroadcast() public {
        vm.chainId(1);
        vm.expectRevert(abi.encodeWithSelector(Deploy.WrongChain.selector, uint256(1)));
        deployment.run();
    }

    function test_rejectsMissingTokenBeforeBroadcast() public {
        vm.chainId(44005);
        vm.expectRevert(abi.encodeWithSelector(Deploy.InvalidToken.selector, TOKEN));
        deployment.run();
    }

    function test_rejectsWrongDecimalsBeforeBroadcast() public {
        vm.chainId(44005);
        vm.etch(TOKEN, hex"00");
        vm.mockCall(TOKEN, abi.encodeWithSignature("decimals()"), abi.encode(uint8(18)));
        vm.expectRevert(abi.encodeWithSelector(Deploy.WrongDecimals.selector, uint8(18)));
        deployment.run();
    }

    function test_deploysOnAevumWithSixDecimalToken() public {
        vm.chainId(44005);
        vm.etch(TOKEN, hex"00");
        vm.mockCall(TOKEN, abi.encodeWithSignature("decimals()"), abi.encode(uint8(6)));
        deployment.run();
    }
}
