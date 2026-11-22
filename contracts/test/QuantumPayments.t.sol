// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Test} from "forge-std/Test.sol";
import {QuantumPayments} from "../src/QuantumPayments.sol";
import {MockUSD8} from "../src/mocks/MockUSD8.sol";
import {Signing} from "./Signing.sol";

contract QuantumPaymentsTest is Test {
    uint256 private constant PRICE = 5e6;
    uint128 private constant PER_TX_CAP = 5e6;
    uint128 private constant DAILY_CAP = 12e6;
    uint256 private constant START_BALANCE = 100e6;

    MockUSD8 private token;
    QuantumPayments private payments;
    address private payer;
    uint256 private payerKey;
    address private owner;
    uint256 private ownerKey;
    address private agent;
    uint256 private agentKey;
    address private merchant;
    address private otherMerchant;
    address private relayer;
    bytes32 private mandateId;

    function setUp() public {
        (payer, payerKey) = makeAddrAndKey("payer");
        (owner, ownerKey) = makeAddrAndKey("owner");
        (agent, agentKey) = makeAddrAndKey("agent");
        merchant = makeAddr("merchant");
        otherMerchant = makeAddr("other-merchant");
        relayer = makeAddr("relayer");

        token = new MockUSD8();
        payments = new QuantumPayments(address(token));
        token.mint(payer, START_BALANCE);
        token.mint(owner, START_BALANCE);
        mandateId = _makeMandate(
            agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 30 days), _oneMerchant(merchant)
        );
    }

    function test_directPay_movesFundsOnceAndEmits() public {
        bytes32 order = keccak256("direct-order");
        _approvePayer(PRICE);
        vm.expectEmit(true, true, true, true, address(payments));
        emit QuantumPayments.InvoicePaid(order, payer, merchant, PRICE, bytes32(0));
        vm.prank(payer);
        payments.pay(order, merchant, PRICE);

        assertTrue(payments.paid(order));
        assertEq(token.balanceOf(merchant), PRICE);
        assertEq(token.balanceOf(payer), START_BALANCE - PRICE);
        _assertClean();
    }

    function test_publicGetters() public view {
        assertEq(address(payments.token()), address(token));
        assertTrue(payments.allowedMerchants(mandateId, merchant));
        assertFalse(payments.allowedMerchants(mandateId, otherMerchant));
        assertEq(payments.ownerNonce(owner), 1);
        assertFalse(payments.paid(keccak256("unpaid-order")));

        (
            address mandateOwner,
            address mandateAgent,
            uint128 perTxCap,
            uint128 dailyCap,
            uint64 expiresAt,
            bool revoked,
            uint128 spentToday,
            uint64 day
        ) = payments.mandates(mandateId);
        assertEq(mandateOwner, owner);
        assertEq(mandateAgent, agent);
        assertEq(perTxCap, PER_TX_CAP);
        assertEq(dailyCap, DAILY_CAP);
        assertGt(expiresAt, block.timestamp);
        assertFalse(revoked);
        assertEq(spentToday, 0);
        assertEq(day, uint64(block.timestamp / 1 days));
    }

    function test_directPay_rejectsZeroAndInvalidArguments() public {
        bytes32 zeroOrder = bytes32(0);
        vm.prank(payer);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.pay(zeroOrder, merchant, PRICE);
        vm.prank(payer);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.pay(keccak256("order"), address(0), PRICE);
        vm.prank(payer);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.pay(keccak256("order"), merchant, 0);
        _assertClean();
    }

    function test_directPay_rejectsMissingApprovalAndSecondPayment() public {
        bytes32 order = keccak256("direct-order");
        vm.prank(payer);
        vm.expectRevert();
        payments.pay(order, merchant, PRICE);

        _approvePayer(2 * PRICE);
        vm.prank(payer);
        payments.pay(order, merchant, PRICE);
        vm.prank(payer);
        vm.expectRevert(QuantumPayments.AlreadyPaid.selector);
        payments.pay(order, merchant, PRICE);
        _assertClean();
    }

    function test_payWithAuthorization_relayerSubmitsAndPayerPaysNoGas() public {
        bytes32 order = keccak256("auth-order");
        uint256 deadline = block.timestamp + 1 hours;
        (uint8 v, bytes32 r, bytes32 s) = _signReceive(order, payer, payerKey, merchant, PRICE, deadline);

        vm.prank(relayer);
        vm.expectEmit(true, true, true, true, address(payments));
        emit QuantumPayments.InvoicePaid(order, payer, merchant, PRICE, bytes32(0));
        payments.payWithAuthorization(order, payer, merchant, PRICE, 0, deadline, v, r, s);

        assertTrue(payments.paid(order));
        assertEq(token.balanceOf(merchant), PRICE);
        assertEq(token.balanceOf(payer), START_BALANCE - PRICE);
        assertTrue(token.authorizationState(payer, payments.authorizationNonce(order, merchant, PRICE)));
        _assertClean();
    }

    function test_payWithAuthorization_nonceBindsOrderMerchantAndAmount() public {
        bytes32 order = keccak256("auth-order");
        uint256 deadline = block.timestamp + 1 hours;
        (uint8 v, bytes32 r, bytes32 s) = _signReceive(order, payer, payerKey, merchant, PRICE, deadline);

        vm.prank(relayer);
        vm.expectRevert(MockUSD8.InvalidSignature.selector);
        payments.payWithAuthorization(keccak256("other-order"), payer, merchant, PRICE, 0, deadline, v, r, s);

        vm.prank(relayer);
        vm.expectRevert(MockUSD8.InvalidSignature.selector);
        payments.payWithAuthorization(order, payer, otherMerchant, PRICE, 0, deadline, v, r, s);

        vm.prank(relayer);
        vm.expectRevert(MockUSD8.InvalidSignature.selector);
        payments.payWithAuthorization(order, payer, merchant, PRICE + 1, 0, deadline, v, r, s);
        _assertClean();
    }

    function test_payWithAuthorization_rejectsReplayAndInvalidWindow() public {
        bytes32 order = keccak256("auth-order");
        uint256 deadline = vm.getBlockTimestamp() + 1 hours;
        (uint8 v, bytes32 r, bytes32 s) = _signReceive(order, payer, payerKey, merchant, PRICE, deadline);

        vm.prank(relayer);
        payments.payWithAuthorization(order, payer, merchant, PRICE, 0, deadline, v, r, s);
        vm.prank(relayer);
        vm.expectRevert(QuantumPayments.AlreadyPaid.selector);
        payments.payWithAuthorization(order, payer, merchant, PRICE, 0, deadline, v, r, s);

        bytes32 expiredOrder = keccak256("expired-order");
        (uint8 ev, bytes32 er, bytes32 es) =
            _signReceive(expiredOrder, payer, payerKey, merchant, PRICE, deadline);
        vm.warp(deadline);
        vm.prank(relayer);
        vm.expectRevert(MockUSD8.AuthorizationExpired.selector);
        payments.payWithAuthorization(expiredOrder, payer, merchant, PRICE, 0, deadline, ev, er, es);

        bytes32 futureOrder = keccak256("future-order");
        uint256 futureDeadline = block.timestamp + 2 hours;
        (uint8 fv, bytes32 fr, bytes32 fs) =
            _signReceive(futureOrder, payer, payerKey, merchant, PRICE, futureDeadline);
        vm.prank(relayer);
        vm.expectRevert(MockUSD8.AuthorizationExpired.selector);
        payments.payWithAuthorization(
            futureOrder, payer, merchant, PRICE, block.timestamp + 1 hours, futureDeadline, fv, fr, fs
        );
        _assertClean();
    }

    function test_payWithAuthorization_requiresRelayerToBeTheRecipient() public {
        bytes32 order = keccak256("auth-order");
        uint256 deadline = block.timestamp + 1 hours;
        (uint8 v, bytes32 r, bytes32 s) = _signReceive(order, payer, payerKey, merchant, PRICE, deadline);
        bytes32 nonce = payments.authorizationNonce(order, merchant, PRICE);

        vm.prank(relayer);
        vm.expectRevert(MockUSD8.InvalidAuthorization.selector);
        token.receiveWithAuthorization(payer, address(payments), PRICE, 0, deadline, nonce, v, r, s);
        _assertClean();
    }

    function test_eip3009TokenNonceCannotBeReused() public {
        bytes32 nonce = keccak256("token-nonce");
        uint256 deadline = block.timestamp + 1 hours;
        Signing.Auth memory auth = Signing.Auth({
            from: payer, to: merchant, value: PRICE, validAfter: 0, validBefore: deadline, nonce: nonce
        });
        (uint8 v, bytes32 r, bytes32 s) = Signing.signReceive(vm, payerKey, token.DOMAIN_SEPARATOR(), auth);

        vm.prank(merchant);
        token.receiveWithAuthorization(payer, merchant, PRICE, 0, deadline, nonce, v, r, s);

        vm.prank(merchant);
        vm.expectRevert(MockUSD8.AuthorizationUsed.selector);
        token.receiveWithAuthorization(payer, merchant, PRICE, 0, deadline, nonce, v, r, s);

        assertEq(token.balanceOf(merchant), PRICE);
        _assertClean();
    }

    function test_crossPathCannotPayTheSameOrder() public {
        bytes32 directFirst = keccak256("direct-first");
        _approvePayer(PRICE);
        vm.prank(payer);
        payments.pay(directFirst, merchant, PRICE);
        uint256 deadline = block.timestamp + 1 hours;
        (uint8 v, bytes32 r, bytes32 s) =
            _signReceive(directFirst, payer, payerKey, merchant, PRICE, deadline);
        vm.prank(relayer);
        vm.expectRevert(QuantumPayments.AlreadyPaid.selector);
        payments.payWithAuthorization(directFirst, payer, merchant, PRICE, 0, deadline, v, r, s);

        bytes32 authFirst = keccak256("auth-first");
        (uint8 av, bytes32 ar, bytes32 asig) =
            _signReceive(authFirst, payer, payerKey, merchant, PRICE, deadline);
        vm.prank(relayer);
        payments.payWithAuthorization(authFirst, payer, merchant, PRICE, 0, deadline, av, ar, asig);
        vm.prank(payer);
        vm.expectRevert(QuantumPayments.AlreadyPaid.selector);
        payments.pay(authFirst, merchant, PRICE);

        bytes32 mandateFirst = keccak256("mandate-first");
        _approveOwner(PRICE);
        vm.prank(agent);
        payments.agentPay(mandateId, mandateFirst, merchant, PRICE);
        vm.prank(payer);
        vm.expectRevert(QuantumPayments.AlreadyPaid.selector);
        payments.pay(mandateFirst, merchant, PRICE);
        _assertClean();
    }

    function test_createMandate_rejectsInvalidArguments() public {
        address[] memory empty = new address[](0);
        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(
            address(0), PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 1 days), _oneMerchant(merchant)
        );

        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(
            owner, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 1 days), _oneMerchant(merchant)
        );

        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(agent, 0, DAILY_CAP, uint64(block.timestamp + 1 days), _oneMerchant(merchant));

        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(
            agent, DAILY_CAP + 1, DAILY_CAP, uint64(block.timestamp + 1 days), _oneMerchant(merchant)
        );

        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(
            agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp - 1), _oneMerchant(merchant)
        );

        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 1 days), empty);

        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(
            agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 1 days), _oneMerchant(address(0))
        );

        address[] memory tooMany = new address[](21);
        for (uint256 index = 0; index < tooMany.length; ++index) {
            tooMany[index] = address(uint160(index + 1));
        }
        vm.prank(owner);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.createMandate(agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 1 days), tooMany);
        _assertClean();
    }

    function test_agentPay_enforcesAgentAllowlistAndEmitsMandateId() public {
        bytes32 order = keccak256("mandate-order");
        _approveOwner(PRICE);
        vm.expectEmit(true, true, true, true, address(payments));
        emit QuantumPayments.InvoicePaid(order, owner, merchant, PRICE, mandateId);
        vm.prank(agent);
        payments.agentPay(mandateId, order, merchant, PRICE);

        assertTrue(payments.paid(order));
        assertEq(token.balanceOf(merchant), PRICE);
        assertEq(token.balanceOf(owner), START_BALANCE - PRICE);
        (,,,,,, uint128 spentToday,) = payments.mandates(mandateId);
        assertEq(spentToday, PRICE);
        _assertClean();

        vm.prank(makeAddr("not-agent"));
        vm.expectRevert(QuantumPayments.NotAgent.selector);
        payments.agentPay(mandateId, keccak256("not-agent-order"), merchant, PRICE);

        vm.prank(agent);
        vm.expectRevert(QuantumPayments.MerchantNotAllowed.selector);
        payments.agentPay(mandateId, keccak256("bad-merchant-order"), otherMerchant, PRICE);
        _assertClean();
    }

    function test_agentPay_enforcesCapsAndResetsDailySpend() public {
        _approveOwner(2 * DAILY_CAP);
        vm.prank(agent);
        payments.agentPay(mandateId, keccak256("mandate-1"), merchant, PER_TX_CAP);
        vm.prank(agent);
        payments.agentPay(mandateId, keccak256("mandate-2"), merchant, PER_TX_CAP);

        vm.prank(agent);
        vm.expectRevert(QuantumPayments.DailyCapExceeded.selector);
        payments.agentPay(mandateId, keccak256("mandate-3"), merchant, PER_TX_CAP);

        vm.prank(agent);
        vm.expectRevert(QuantumPayments.PerTxCapExceeded.selector);
        payments.agentPay(mandateId, keccak256("mandate-4"), merchant, PER_TX_CAP + 1);

        vm.warp(block.timestamp + 1 days);
        vm.prank(agent);
        payments.agentPay(mandateId, keccak256("mandate-5"), merchant, PER_TX_CAP);
        (,,,,,, uint128 spentToday,) = payments.mandates(mandateId);
        assertEq(spentToday, PER_TX_CAP);
        _assertClean();
    }

    function test_agentPay_expiryRevokeAndOwnerChecks() public {
        _approveOwner(DAILY_CAP);
        vm.prank(makeAddr("not-owner"));
        vm.expectRevert(QuantumPayments.NotOwner.selector);
        payments.revokeMandate(mandateId);

        vm.prank(agent);
        payments.agentPay(mandateId, keccak256("before-revoke"), merchant, PRICE);
        vm.prank(owner);
        payments.revokeMandate(mandateId);

        vm.prank(agent);
        vm.expectRevert(QuantumPayments.MandateIsRevoked.selector);
        payments.agentPay(mandateId, keccak256("after-revoke"), merchant, PRICE);
        vm.prank(owner);
        vm.expectRevert(QuantumPayments.MandateIsRevoked.selector);
        payments.revokeMandate(mandateId);

        bytes32 expiring =
            _makeMandate(agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 10), _oneMerchant(merchant));
        vm.warp(block.timestamp + 11);
        vm.prank(agent);
        vm.expectRevert(QuantumPayments.MandateExpired.selector);
        payments.agentPay(expiring, keccak256("expired-order"), merchant, PRICE);
        _assertClean();
    }

    function test_agentPay_requiresSufficientAllowanceAndBalance() public {
        _approveOwner(PRICE - 1);
        vm.prank(agent);
        vm.expectRevert();
        payments.agentPay(mandateId, keccak256("low-allowance"), merchant, PRICE);
        _assertClean();

        _approveOwner(PRICE);
        vm.prank(owner);
        token.transfer(makeAddr("sink"), START_BALANCE - PRICE + 1);
        vm.prank(agent);
        vm.expectRevert();
        payments.agentPay(mandateId, keccak256("low-balance"), merchant, PRICE);
        _assertClean();
    }

    function test_approval_requestApproveRejectAndExpiry() public {
        bytes32 order = keccak256("approval-order");
        uint256 amount = 8e6;
        _approveOwner(amount);
        bytes32 requestId = _requestApproval(mandateId, order, merchant, amount);
        (,,,, bool pending) = payments.approvals(requestId);
        assertTrue(pending);

        vm.prank(makeAddr("not-owner"));
        vm.expectRevert(QuantumPayments.NotOwner.selector);
        payments.approveRequest(requestId);

        vm.expectEmit(true, true, false, true, address(payments));
        emit QuantumPayments.ApprovalResolved(requestId, mandateId, true);
        vm.prank(owner);
        payments.approveRequest(requestId);

        assertTrue(payments.paid(order));
        assertEq(token.balanceOf(merchant), amount);
        (,,,, pending) = payments.approvals(requestId);
        assertFalse(pending);
        (,,,,,, uint128 spentToday,) = payments.mandates(mandateId);
        assertEq(spentToday, 0);
        _assertClean();

        bytes32 rejectedOrder = keccak256("rejected-order");
        bytes32 rejectedId = _requestApproval(mandateId, rejectedOrder, merchant, PRICE);
        vm.prank(owner);
        payments.rejectRequest(rejectedId);
        vm.prank(owner);
        vm.expectRevert(QuantumPayments.RequestNotPending.selector);
        payments.approveRequest(rejectedId);
        assertFalse(payments.paid(rejectedOrder));
        _assertClean();
    }

    function test_approval_cannotBeApprovedAfterRevokeOrExpiry() public {
        bytes32 revokedOrder = keccak256("revoked-approval");
        bytes32 revokedRequest = _requestApproval(mandateId, revokedOrder, merchant, PRICE);
        vm.prank(owner);
        payments.revokeMandate(mandateId);
        vm.prank(owner);
        vm.expectRevert(QuantumPayments.MandateIsRevoked.selector);
        payments.approveRequest(revokedRequest);

        bytes32 expiring =
            _makeMandate(agent, PER_TX_CAP, DAILY_CAP, uint64(block.timestamp + 10), _oneMerchant(merchant));
        bytes32 expiringOrder = keccak256("expiring-approval");
        bytes32 expiringRequest = _requestApproval(expiring, expiringOrder, merchant, PRICE);
        vm.warp(block.timestamp + 11);
        vm.prank(owner);
        vm.expectRevert(QuantumPayments.MandateExpired.selector);
        payments.approveRequest(expiringRequest);
        vm.prank(agent);
        vm.expectRevert(QuantumPayments.MandateExpired.selector);
        payments.requestApproval(expiring, keccak256("late-request"), merchant, PRICE);
        _assertClean();
    }

    function test_approval_requiresAgentAndAllowlistedMerchant() public {
        vm.prank(makeAddr("not-agent"));
        vm.expectRevert(QuantumPayments.NotAgent.selector);
        payments.requestApproval(mandateId, keccak256("bad-agent-request"), merchant, PRICE);

        vm.prank(agent);
        vm.expectRevert(QuantumPayments.MerchantNotAllowed.selector);
        payments.requestApproval(mandateId, keccak256("bad-merchant-request"), otherMerchant, PRICE);

        vm.prank(agent);
        vm.expectRevert(QuantumPayments.InvalidArgument.selector);
        payments.requestApproval(mandateId, keccak256("bad-amount-request"), merchant, 0);
        _assertClean();
    }

    function testFuzz_agentPay_dailySpendNeverExceedsCap(uint256 first, uint256 second, uint64 timeOffset)
        public
    {
        bytes32 id = _makeMandate(
            agent, DAILY_CAP, DAILY_CAP, uint64(vm.getBlockTimestamp() + 100 days), _oneMerchant(merchant)
        );
        _approveOwner(2 * DAILY_CAP);
        first = bound(first, 1, DAILY_CAP);
        second = bound(second, 1, DAILY_CAP);
        timeOffset = uint64(bound(timeOffset, 0, 23 hours));
        vm.warp(vm.getBlockTimestamp() + timeOffset);

        vm.prank(agent);
        payments.agentPay(id, keccak256(abi.encode("fuzz-first", first)), merchant, first);
        (,,,,,, uint128 spentAfterFirst,) = payments.mandates(id);
        assertEq(spentAfterFirst, uint128(first));

        bytes32 secondOrder = keccak256(abi.encode("fuzz-second", first, second));
        if (first + second <= DAILY_CAP) {
            vm.prank(agent);
            payments.agentPay(id, secondOrder, merchant, second);
            (,,,,,, uint128 spentAfterSecond,) = payments.mandates(id);
            assertEq(spentAfterSecond, uint128(first + second));
            assertLe(spentAfterSecond, DAILY_CAP);
        } else {
            vm.expectRevert(QuantumPayments.DailyCapExceeded.selector);
            vm.prank(agent);
            payments.agentPay(id, secondOrder, merchant, second);
            (,,,,,, uint128 spentAfterRevert,) = payments.mandates(id);
            assertEq(spentAfterRevert, uint128(first));
        }
        _assertClean();
    }

    function testFuzz_agentPay_resetsSpentTodayAfterDayChange(uint128 first, uint128 second, uint64 offset)
        public
    {
        first = uint128(bound(first, 1, PER_TX_CAP));
        second = uint128(bound(second, 1, PER_TX_CAP));
        offset = uint64(bound(offset, 0, 1 days - 1));
        _approveOwner(2 * DAILY_CAP);

        vm.prank(agent);
        payments.agentPay(mandateId, keccak256(abi.encode("first", first)), merchant, first);
        vm.warp(block.timestamp + 1 days + offset);
        vm.prank(agent);
        payments.agentPay(mandateId, keccak256(abi.encode("second", second)), merchant, second);

        (,,,,,, uint128 spentToday,) = payments.mandates(mandateId);
        assertEq(spentToday, second);
        _assertClean();
    }

    function _approvePayer(uint256 amount) private {
        vm.prank(payer);
        token.approve(address(payments), amount);
    }

    function _approveOwner(uint256 amount) private {
        vm.prank(owner);
        token.approve(address(payments), amount);
    }

    function _oneMerchant(address target) private pure returns (address[] memory merchants) {
        merchants = new address[](1);
        merchants[0] = target;
    }

    function _makeMandate(
        address agent_,
        uint128 perTxCap,
        uint128 dailyCap,
        uint64 expiresAt,
        address[] memory merchants
    ) private returns (bytes32 id) {
        uint256 nonce = payments.ownerNonce(owner);
        id = keccak256(abi.encode(block.chainid, address(payments), owner, agent_, nonce));
        vm.prank(owner);
        payments.createMandate(agent_, perTxCap, dailyCap, expiresAt, merchants);
    }

    function _signReceive(
        bytes32 order,
        address from,
        uint256 key,
        address merchant_,
        uint256 amount,
        uint256 deadline
    ) private view returns (uint8 v, bytes32 r, bytes32 s) {
        Signing.Auth memory auth = Signing.Auth({
            from: from,
            to: address(payments),
            value: amount,
            validAfter: 0,
            validBefore: deadline,
            nonce: payments.authorizationNonce(order, merchant_, amount)
        });
        return Signing.signReceive(vm, key, token.DOMAIN_SEPARATOR(), auth);
    }

    function _requestApproval(bytes32 id, bytes32 order, address merchant_, uint256 amount)
        private
        returns (bytes32 requestId)
    {
        requestId = keccak256(abi.encode(block.chainid, address(payments), id, order, merchant_, amount));
        vm.prank(agent);
        payments.requestApproval(id, order, merchant_, amount);
    }

    function _assertClean() private view {
        assertEq(token.balanceOf(address(payments)), 0, "contract must not hold tokens");
    }
}
