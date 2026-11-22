// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {QuantumPayments} from "../../src/QuantumPayments.sol";
import {Signing} from "../Signing.sol";

// Adapted from qday-io/aevum-starter-kit (MIT), Copyright (c) 2026 PQA Labs.
interface ITUSD8 {
    function DOMAIN_SEPARATOR() external view returns (bytes32);
    function authorizationState(address, bytes32) external view returns (bool);
    function balanceOf(address) external view returns (uint256);
    function approve(address, uint256) external returns (bool);
}

// Adapted from qday-io/aevum-starter-kit (MIT), Copyright (c) 2026 PQA Labs.
contract AevumForkTest is Test {
    ITUSD8 private constant TUSD8 = ITUSD8(0x456569838f1d43009A91ebA4263147Afd7233483);
    uint256 private constant AMOUNT = 1e6;

    function test_realTokenDomainAndShanghaiDeployment() public {
        require(block.chainid == 44005, "run with --fork-url aevum");
        bytes32 expected = keccak256(
            abi.encode(
                keccak256(
                    "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
                ),
                keccak256("tUSD8"),
                keccak256("2"),
                uint256(44005),
                address(TUSD8)
            )
        );
        assertEq(TUSD8.DOMAIN_SEPARATOR(), expected);
        QuantumPayments payments = new QuantumPayments(address(TUSD8));
        assertTrue(address(payments).code.length > 0, "Shanghai deployment succeeded on Aevum");
    }

    function test_realTokenPayWithAuthorization() public {
        require(block.chainid == 44005, "run with --fork-url aevum");
        (address payer, uint256 key) = makeAddrAndKey("fork-payer");
        address merchant = makeAddr("fork-merchant");
        address relayer = makeAddr("fork-relayer");
        QuantumPayments payments = new QuantumPayments(address(TUSD8));
        deal(address(TUSD8), payer, 10e6);

        bytes32 order = keccak256("fork-auth-order");
        uint256 deadline = block.timestamp + 1 hours;
        Signing.Auth memory auth = Signing.Auth({
            from: payer,
            to: address(payments),
            value: AMOUNT,
            validAfter: 0,
            validBefore: deadline,
            nonce: payments.authorizationNonce(order, merchant, AMOUNT)
        });
        (uint8 v, bytes32 r, bytes32 s) = Signing.signReceive(vm, key, TUSD8.DOMAIN_SEPARATOR(), auth);

        vm.prank(relayer);
        payments.payWithAuthorization(order, payer, merchant, AMOUNT, 0, deadline, v, r, s);

        assertEq(TUSD8.balanceOf(merchant), AMOUNT);
        assertEq(TUSD8.balanceOf(payer), 9e6);
        assertTrue(TUSD8.authorizationState(payer, payments.authorizationNonce(order, merchant, AMOUNT)));
        assertTrue(payments.paid(order));
        assertEq(TUSD8.balanceOf(address(payments)), 0);
    }

    function test_realTokenApproveAndPay() public {
        require(block.chainid == 44005, "run with --fork-url aevum");
        address payer = makeAddr("fork-direct-payer");
        address merchant = makeAddr("fork-direct-merchant");
        QuantumPayments payments = new QuantumPayments(address(TUSD8));
        deal(address(TUSD8), payer, 10e6);

        vm.startPrank(payer);
        TUSD8.approve(address(payments), AMOUNT);
        payments.pay(keccak256("fork-direct-order"), merchant, AMOUNT);
        vm.stopPrank();

        assertEq(TUSD8.balanceOf(merchant), AMOUNT);
        assertEq(TUSD8.balanceOf(address(payments)), 0);
    }
}
