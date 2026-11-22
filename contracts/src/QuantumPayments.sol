// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

interface IEIP3009 {
    function receiveWithAuthorization(
        address from,
        address to,
        uint256 value,
        uint256 validAfter,
        uint256 validBefore,
        bytes32 nonce,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external;
}

/// @title QuantumPayments
/// @notice Noncustodial testnet invoice settlement and on-chain agent spend mandates.
/// @dev Every payment moves tokens directly from the payer to the merchant and emits one
///      `InvoicePaid` event. The contract has no owner, no upgrade path and no held balance.
contract QuantumPayments is ReentrancyGuard {
    using SafeERC20 for IERC20;

    struct Mandate {
        address owner;
        address agent;
        uint128 perTxCap;
        uint128 dailyCap;
        uint64 expiresAt;
        bool revoked;
        uint128 spentToday;
        uint64 day;
    }

    struct ApprovalRequest {
        bytes32 mandateId;
        bytes32 orderId;
        address merchant;
        uint256 amount;
        bool pending;
    }

    IERC20 public immutable token;

    mapping(bytes32 => bool) public paid;
    mapping(address => uint256) public ownerNonce;
    mapping(bytes32 => Mandate) public mandates;
    mapping(bytes32 => mapping(address => bool)) public allowedMerchants;
    mapping(bytes32 => ApprovalRequest) public approvals;

    event InvoicePaid(
        bytes32 indexed orderId,
        address indexed payer,
        address indexed merchant,
        uint256 amount,
        bytes32 mandateId
    );
    event MandateCreated(
        bytes32 indexed mandateId,
        address indexed owner,
        address indexed agent,
        uint128 perTxCap,
        uint128 dailyCap,
        uint64 expiresAt
    );
    event MandateRevoked(bytes32 indexed mandateId, address indexed owner);
    event ApprovalRequested(
        bytes32 indexed requestId,
        bytes32 indexed mandateId,
        address indexed agent,
        bytes32 orderId,
        address merchant,
        uint256 amount
    );
    event ApprovalResolved(bytes32 indexed requestId, bytes32 indexed mandateId, bool approved);

    error AlreadyPaid();
    error NotAgent();
    error NotOwner();
    error MandateIsRevoked();
    error MandateExpired();
    error MerchantNotAllowed();
    error PerTxCapExceeded();
    error DailyCapExceeded();
    error InvalidArgument();
    error RequestNotPending();

    constructor(address token_) {
        if (token_ == address(0)) revert InvalidArgument();
        token = IERC20(token_);
    }

    /// @notice Pay an invoice with tokens already approved to this contract by the caller.
    /// @param orderId Opaque invoice identifier; must be unique and non-zero.
    /// @param merchant Receiver of the tokens; must be non-zero.
    /// @param amount Token amount in base units; must be positive.
    function pay(bytes32 orderId, address merchant, uint256 amount) external nonReentrant {
        _requirePayment(orderId, merchant, amount);
        _pullAndRecord(msg.sender, orderId, merchant, amount, bytes32(0));
    }

    /// @notice Pay an invoice gaslessly with an EIP-3009 authorization signed by the payer.
    /// @dev The signed nonce binds the authorization to this contract, chain, order, merchant and amount.
    /// @param orderId Opaque invoice identifier; must be unique and non-zero.
    /// @param payer Token owner that signed the EIP-3009 authorization.
    /// @param merchant Receiver of the tokens; must be non-zero.
    /// @param amount Token amount in base units; must be positive.
    /// @param validAfter Earliest Unix timestamp at which the authorization is valid.
    /// @param validBefore Expiry Unix timestamp of the authorization.
    /// @param v ECDSA recovery byte of the payer's signature.
    /// @param r ECDSA r value of the payer's signature.
    /// @param s ECDSA s value of the payer's signature.
    function payWithAuthorization(
        bytes32 orderId,
        address payer,
        address merchant,
        uint256 amount,
        uint256 validAfter,
        uint256 validBefore,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external nonReentrant {
        _requirePayment(orderId, merchant, amount);
        if (payer == address(0)) revert InvalidArgument();
        bytes32 nonce = keccak256(abi.encode(block.chainid, address(this), orderId, merchant, amount));
        uint256 balanceBefore = token.balanceOf(address(this));
        _recordPayment(orderId, payer, merchant, amount, bytes32(0));
        IEIP3009(address(token))
            .receiveWithAuthorization(payer, address(this), amount, validAfter, validBefore, nonce, v, r, s);
        token.safeTransfer(merchant, amount);
        assert(token.balanceOf(address(this)) == balanceBefore);
    }

    /// @notice Compute the EIP-3009 nonce for an invoice payment through this contract.
    /// @param orderId Opaque invoice identifier.
    /// @param merchant Receiver of the tokens.
    /// @param amount Token amount in base units.
    /// @return nonce The nonce a payer must sign.
    function authorizationNonce(bytes32 orderId, address merchant, uint256 amount)
        public
        view
        returns (bytes32 nonce)
    {
        return keccak256(abi.encode(block.chainid, address(this), orderId, merchant, amount));
    }

    /// @notice Create a scoped on-chain spending mandate for an agent.
    /// @param agent Address allowed to spend under this mandate.
    /// @param perTxCap Maximum base units for one agent payment.
    /// @param dailyCap Maximum base units per UTC day; must be at least `perTxCap`.
    /// @param expiresAt Unix expiry timestamp; must be in the future.
    /// @param merchants Non-empty allowlist of at most 20 receivers.
    /// @return mandateId Unique identifier of the new mandate.
    function createMandate(
        address agent,
        uint128 perTxCap,
        uint128 dailyCap,
        uint64 expiresAt,
        address[] calldata merchants
    ) external returns (bytes32 mandateId) {
        if (agent == address(0) || agent == msg.sender) revert InvalidArgument();
        if (perTxCap == 0 || perTxCap > dailyCap) revert InvalidArgument();
        if (uint256(expiresAt) <= block.timestamp) revert InvalidArgument();
        if (merchants.length == 0 || merchants.length > 20) revert InvalidArgument();

        mandateId =
            keccak256(abi.encode(block.chainid, address(this), msg.sender, agent, ownerNonce[msg.sender]++));
        Mandate storage mandate = mandates[mandateId];
        mandate.owner = msg.sender;
        mandate.agent = agent;
        mandate.perTxCap = perTxCap;
        mandate.dailyCap = dailyCap;
        mandate.expiresAt = expiresAt;
        mandate.day = uint64(block.timestamp / 1 days);

        for (uint256 index = 0; index < merchants.length; ++index) {
            address merchant = merchants[index];
            if (merchant == address(0)) revert InvalidArgument();
            allowedMerchants[mandateId][merchant] = true;
        }

        emit MandateCreated(mandateId, msg.sender, agent, perTxCap, dailyCap, expiresAt);
    }

    /// @notice Revoke a mandate immediately and permanently.
    /// @param mandateId Identifier of the caller's mandate.
    function revokeMandate(bytes32 mandateId) external {
        Mandate storage mandate = mandates[mandateId];
        if (mandate.owner == address(0)) revert InvalidArgument();
        if (msg.sender != mandate.owner) revert NotOwner();
        if (mandate.revoked) revert MandateIsRevoked();
        mandate.revoked = true;
        emit MandateRevoked(mandateId, msg.sender);
    }

    /// @notice Spend under an active mandate within its per-transaction and daily caps.
    /// @param mandateId Identifier of the mandate.
    /// @param orderId Opaque invoice identifier; must be unique and non-zero.
    /// @param merchant Allowlisted receiver of the tokens.
    /// @param amount Token amount in base units; must be positive and within both caps.
    function agentPay(bytes32 mandateId, bytes32 orderId, address merchant, uint256 amount)
        external
        nonReentrant
    {
        Mandate storage mandate = _activeMandate(mandateId);
        if (msg.sender != mandate.agent) revert NotAgent();
        _requireAllowlisted(mandateId, merchant);
        _requirePayment(orderId, merchant, amount);
        if (amount > uint256(mandate.perTxCap)) revert PerTxCapExceeded();

        _resetDayIfNeeded(mandate);
        uint256 nextSpent = uint256(mandate.spentToday) + amount;
        if (nextSpent > uint256(mandate.dailyCap)) revert DailyCapExceeded();
        mandate.spentToday = uint128(nextSpent);

        _pullAndRecord(mandate.owner, orderId, merchant, amount, mandateId);
    }

    /// @notice Ask the mandate owner to approve a payment above the agent's automatic caps.
    /// @param mandateId Identifier of the mandate.
    /// @param orderId Opaque invoice identifier; must be unique and non-zero.
    /// @param merchant Allowlisted receiver of the tokens.
    /// @param amount Token amount in base units; must be positive.
    /// @return requestId Unique identifier of the approval request.
    function requestApproval(bytes32 mandateId, bytes32 orderId, address merchant, uint256 amount)
        external
        returns (bytes32 requestId)
    {
        Mandate storage mandate = _activeMandate(mandateId);
        if (msg.sender != mandate.agent) revert NotAgent();
        _requireAllowlisted(mandateId, merchant);
        _requirePayment(orderId, merchant, amount);

        requestId = keccak256(abi.encode(block.chainid, address(this), mandateId, orderId, merchant, amount));
        if (approvals[requestId].pending) revert InvalidArgument();
        ApprovalRequest storage request = approvals[requestId];
        request.mandateId = mandateId;
        request.orderId = orderId;
        request.merchant = merchant;
        request.amount = amount;
        request.pending = true;

        emit ApprovalRequested(requestId, mandateId, msg.sender, orderId, merchant, amount);
    }

    /// @notice Approve a pending request and execute its payment, bypassing the automatic caps.
    /// @dev Approved payments do not increase `spentToday`; they are exceptional, human-authorized spends.
    /// @param requestId Identifier of the pending approval request.
    function approveRequest(bytes32 requestId) external nonReentrant {
        ApprovalRequest storage request = approvals[requestId];
        if (!request.pending) revert RequestNotPending();
        Mandate storage mandate = mandates[request.mandateId];
        if (msg.sender != mandate.owner) revert NotOwner();
        if (mandate.revoked) revert MandateIsRevoked();
        if (block.timestamp >= uint256(mandate.expiresAt)) revert MandateExpired();
        _requireAllowlisted(request.mandateId, request.merchant);

        request.pending = false;
        emit ApprovalResolved(requestId, request.mandateId, true);
        _pullAndRecord(mandate.owner, request.orderId, request.merchant, request.amount, request.mandateId);
    }

    /// @notice Reject a pending approval request without moving tokens.
    /// @param requestId Identifier of the pending approval request.
    function rejectRequest(bytes32 requestId) external {
        ApprovalRequest storage request = approvals[requestId];
        if (!request.pending) revert RequestNotPending();
        Mandate storage mandate = mandates[request.mandateId];
        if (msg.sender != mandate.owner) revert NotOwner();
        request.pending = false;
        emit ApprovalResolved(requestId, request.mandateId, false);
    }

    function _activeMandate(bytes32 mandateId) private view returns (Mandate storage mandate) {
        mandate = mandates[mandateId];
        if (mandate.owner == address(0)) revert InvalidArgument();
        if (mandate.revoked) revert MandateIsRevoked();
        if (block.timestamp >= uint256(mandate.expiresAt)) revert MandateExpired();
    }

    function _requirePayment(bytes32 orderId, address merchant, uint256 amount) private view {
        if (orderId == bytes32(0) || merchant == address(0) || amount == 0) revert InvalidArgument();
        if (paid[orderId]) revert AlreadyPaid();
    }

    function _requireAllowlisted(bytes32 mandateId, address merchant) private view {
        if (!allowedMerchants[mandateId][merchant]) revert MerchantNotAllowed();
    }

    function _resetDayIfNeeded(Mandate storage mandate) private {
        uint64 today = uint64(block.timestamp / 1 days);
        if (mandate.day != today) {
            mandate.day = today;
            mandate.spentToday = 0;
        }
    }

    function _recordPayment(
        bytes32 orderId,
        address payer,
        address merchant,
        uint256 amount,
        bytes32 mandateId
    ) private {
        paid[orderId] = true;
        emit InvoicePaid(orderId, payer, merchant, amount, mandateId);
    }

    function _pullAndRecord(
        address payer,
        bytes32 orderId,
        address merchant,
        uint256 amount,
        bytes32 mandateId
    ) private {
        uint256 balanceBefore = token.balanceOf(address(this));
        _recordPayment(orderId, payer, merchant, amount, mandateId);
        token.safeTransferFrom(payer, merchant, amount);
        assert(token.balanceOf(address(this)) == balanceBefore);
    }
}
