package blockchain

import (
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"paycore/api/internal/models"
)

func decodeInvoicePaid(log *types.Log) (models.QuantumInvoicePaid, bool) {
	topic := crypto.Keccak256Hash([]byte("InvoicePaid(bytes32,address,address,uint256,bytes32)"))
	if log.Removed || len(log.Topics) != 4 || log.Topics[0] != topic || len(log.Data) != 64 || uint64(log.Index) > math.MaxInt64 {
		return models.QuantumInvoicePaid{}, false
	}
	if new(big.Int).SetBytes(log.Topics[2][:12]).Sign() != 0 || new(big.Int).SetBytes(log.Topics[3][:12]).Sign() != 0 {
		return models.QuantumInvoicePaid{}, false
	}
	amount := new(big.Int).SetBytes(log.Data[:32])
	event := models.QuantumInvoicePaid{
		Contract: strings.ToLower(log.Address.Hex()), OrderID: strings.ToLower(log.Topics[1].Hex()),
		Payer:    strings.ToLower(common.BytesToAddress(log.Topics[2][12:]).Hex()),
		Merchant: strings.ToLower(common.BytesToAddress(log.Topics[3][12:]).Hex()),
		Overflow: !amount.IsInt64(), LogIndex: uint64(log.Index),
	}
	if !event.Overflow {
		event.Amount = amount.Int64()
	}
	return event, true
}
