// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

contract MockUSD8 is IERC20 {
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    bytes32 private constant PERMIT_TYPEHASH =
        keccak256("Permit(address owner,address spender,uint256 value,uint256 nonce,uint256 deadline)");
    bytes32 private constant RECEIVE_TYPEHASH = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 private constant TRANSFER_TYPEHASH = keccak256(
        "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );

    string public constant name = "Mock tUSD8";
    string public constant symbol = "tUSD8";
    string public constant version = "2";
    uint8 public constant decimals = 6;
    bytes32 public immutable DOMAIN_SEPARATOR;

    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;
    mapping(address => uint256) public nonces;
    mapping(address => mapping(bytes32 => bool)) public authorizationState;

    error InvalidSignature();
    error AuthorizationExpired();
    error AuthorizationUsed();
    error InvalidAuthorization();

    constructor() {
        DOMAIN_SEPARATOR = keccak256(
            abi.encode(DOMAIN_TYPEHASH, keccak256("tUSD8"), keccak256("2"), block.chainid, address(this))
        );
    }

    function mint(address account, uint256 value) external {
        if (account == address(0)) revert InvalidAuthorization();
        totalSupply += value;
        balanceOf[account] += value;
        emit Transfer(address(0), account, value);
    }

    function approve(address spender, uint256 value) external returns (bool) {
        allowance[msg.sender][spender] = value;
        emit Approval(msg.sender, spender, value);
        return true;
    }

    function transfer(address to, uint256 value) external returns (bool) {
        _transfer(msg.sender, to, value);
        return true;
    }

    function transferFrom(address from, address to, uint256 value) external returns (bool) {
        uint256 currentAllowance = allowance[from][msg.sender];
        if (currentAllowance < value) revert InvalidAuthorization();
        if (currentAllowance != type(uint256).max) allowance[from][msg.sender] = currentAllowance - value;
        _transfer(from, to, value);
        return true;
    }

    function permit(
        address owner,
        address spender,
        uint256 value,
        uint256 deadline,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external {
        if (block.timestamp > deadline) revert AuthorizationExpired();
        bytes32 structHash =
            keccak256(abi.encode(PERMIT_TYPEHASH, owner, spender, value, nonces[owner]++, deadline));
        address signer = _recover(structHash, v, r, s);
        if (signer != owner) revert InvalidSignature();
        allowance[owner][spender] = value;
        emit Approval(owner, spender, value);
    }

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
    ) external {
        if (msg.sender != to) revert InvalidAuthorization();
        address signer =
            _authorize(from, to, value, validAfter, validBefore, nonce, RECEIVE_TYPEHASH, v, r, s);
        if (signer != from) revert InvalidSignature();
        _transfer(from, to, value);
    }

    function transferWithAuthorization(
        address from,
        address to,
        uint256 value,
        uint256 validAfter,
        uint256 validBefore,
        bytes32 nonce,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external {
        address signer = _authorize(
            from, to, value, validAfter, validBefore, nonce, TRANSFER_TYPEHASH, v, r, s
        );
        if (signer != from) revert InvalidSignature();
        _transfer(from, to, value);
    }

    function _authorize(
        address from,
        address to,
        uint256 value,
        uint256 validAfter,
        uint256 validBefore,
        bytes32 nonce,
        bytes32 typeHash,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) private returns (address signer) {
        if (from == address(0) || to == address(0) || value == 0) revert InvalidAuthorization();
        if (block.timestamp <= validAfter || block.timestamp >= validBefore) revert AuthorizationExpired();
        if (authorizationState[from][nonce]) revert AuthorizationUsed();
        authorizationState[from][nonce] = true;
        bytes32 structHash = keccak256(abi.encode(typeHash, from, to, value, validAfter, validBefore, nonce));
        signer = _recover(structHash, v, r, s);
        if (signer == address(0)) revert InvalidSignature();
    }

    function _recover(bytes32 structHash, uint8 v, bytes32 r, bytes32 s) private view returns (address) {
        bytes32 digest = keccak256(abi.encodePacked("\x19\x01", DOMAIN_SEPARATOR, structHash));
        return ecrecover(digest, v, r, s);
    }

    function _transfer(address from, address to, uint256 value) private {
        if (to == address(0)) revert InvalidAuthorization();
        uint256 fromBalance = balanceOf[from];
        if (fromBalance < value) revert InvalidAuthorization();
        unchecked {
            balanceOf[from] = fromBalance - value;
            balanceOf[to] += value;
        }
        emit Transfer(from, to, value);
    }
}
