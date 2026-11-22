// SPDX-License-Identifier: MIT
pragma solidity 0.8.24;

import {Vm} from "forge-std/Vm.sol";

// Adapted from qday-io/aevum-starter-kit (MIT), Copyright (c) 2026 PQA Labs.
library Signing {
    bytes32 internal constant RECEIVE_TYPEHASH = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 internal constant TRANSFER_TYPEHASH = keccak256(
        "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 internal constant PERMIT_TYPEHASH =
        keccak256("Permit(address owner,address spender,uint256 value,uint256 nonce,uint256 deadline)");

    struct Auth {
        address from;
        address to;
        uint256 value;
        uint256 validAfter;
        uint256 validBefore;
        bytes32 nonce;
    }

    function signReceive(Vm vm, uint256 privateKey, bytes32 domainSeparator, Auth memory auth)
        internal
        pure
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        bytes32 structHash = keccak256(
            abi.encode(
                RECEIVE_TYPEHASH,
                auth.from,
                auth.to,
                auth.value,
                auth.validAfter,
                auth.validBefore,
                auth.nonce
            )
        );
        return _sign(vm, privateKey, domainSeparator, structHash);
    }

    function signTransfer(Vm vm, uint256 privateKey, bytes32 domainSeparator, Auth memory auth)
        internal
        pure
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        bytes32 structHash = keccak256(
            abi.encode(
                TRANSFER_TYPEHASH,
                auth.from,
                auth.to,
                auth.value,
                auth.validAfter,
                auth.validBefore,
                auth.nonce
            )
        );
        return _sign(vm, privateKey, domainSeparator, structHash);
    }

    function signPermit(
        Vm vm,
        uint256 privateKey,
        bytes32 domainSeparator,
        address owner,
        address spender,
        uint256 value,
        uint256 nonce,
        uint256 deadline
    ) internal pure returns (uint8 v, bytes32 r, bytes32 s) {
        bytes32 structHash = keccak256(abi.encode(PERMIT_TYPEHASH, owner, spender, value, nonce, deadline));
        return _sign(vm, privateKey, domainSeparator, structHash);
    }

    function _sign(Vm vm, uint256 privateKey, bytes32 domainSeparator, bytes32 structHash)
        private
        pure
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        return vm.sign(privateKey, keccak256(abi.encodePacked("\x19\x01", domainSeparator, structHash)));
    }
}
