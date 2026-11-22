// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {QuantumPayments} from "../src/QuantumPayments.sol";
import {MockUSD8} from "../src/mocks/MockUSD8.sol";
import {Signing} from "./Signing.sol";

contract QuantumPaymentsHandler is Test {
    uint256 private constant AMOUNT = 1e6;
    uint256 private counter;
    MockUSD8 private token;
    QuantumPayments private payments;
    address private payer;
    uint256 private payerKey;
    address private agent;
    address private merchant;
    address private relayer;
    bytes32 private mandateId;

    constructor(
        MockUSD8 token_,
        QuantumPayments payments_,
        address payer_,
        uint256 payerKey_,
        address agent_,
        address merchant_,
        address relayer_,
        bytes32 mandateId_
    ) {
        token = token_;
        payments = payments_;
        payer = payer_;
        payerKey = payerKey_;
        agent = agent_;
        merchant = merchant_;
        relayer = relayer_;
        mandateId = mandateId_;
    }

    function directPay(uint256 amount) public {
        amount = bound(amount, 1, AMOUNT);
        vm.prank(payer);
        token.approve(address(payments), amount);
        vm.prank(payer);
        payments.pay(_nextOrder("direct"), merchant, amount);
    }

    function authorizedPay(uint256 amount) public {
        amount = bound(amount, 1, AMOUNT);
        bytes32 order = _nextOrder("auth");
        uint256 deadline = block.timestamp + 1 hours;
        Signing.Auth memory auth = Signing.Auth({
            from: payer,
            to: address(payments),
            value: amount,
            validAfter: 0,
            validBefore: deadline,
            nonce: payments.authorizationNonce(order, merchant, amount)
        });
        (uint8 v, bytes32 r, bytes32 s) = Signing.signReceive(vm, payerKey, token.DOMAIN_SEPARATOR(), auth);
        vm.prank(relayer);
        payments.payWithAuthorization(order, payer, merchant, amount, 0, deadline, v, r, s);
    }

    function mandatePay(uint256 amount) public {
        amount = bound(amount, 1, AMOUNT);
        vm.prank(agent);
        payments.agentPay(mandateId, _nextOrder("mandate"), merchant, amount);
    }

    function _nextOrder(string memory prefix) private returns (bytes32) {
        return keccak256(abi.encodePacked(prefix, counter++));
    }
}

contract QuantumPaymentsInvariantTest is Test {
    uint256 private constant AMOUNT = 1e6;
    MockUSD8 private token;
    QuantumPayments private payments;

    function setUp() public {
        (address payer, uint256 payerKey) = makeAddrAndKey("payer");
        address owner = makeAddr("owner");
        address agent = makeAddr("agent");
        address merchant = makeAddr("merchant");
        address relayer = makeAddr("relayer");

        token = new MockUSD8();
        payments = new QuantumPayments(address(token));
        token.mint(payer, 100e6);
        token.mint(owner, 100e6);

        address[] memory merchants = new address[](1);
        merchants[0] = merchant;
        vm.prank(owner);
        bytes32 mandateId = payments.createMandate(
            agent, uint128(AMOUNT), type(uint128).max, uint64(block.timestamp + 100 days), merchants
        );
        vm.prank(owner);
        token.approve(address(payments), type(uint256).max);

        QuantumPaymentsHandler handler =
            new QuantumPaymentsHandler(token, payments, payer, payerKey, agent, merchant, relayer, mandateId);
        targetContract(address(handler));
    }

    function invariant_contractHoldsNoTokens() public view {
        assertEq(token.balanceOf(address(payments)), 0, "QuantumPayments must never hold tokens");
    }
}
