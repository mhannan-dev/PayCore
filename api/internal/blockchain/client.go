// Package blockchain is the only adapter importing go-ethereum. No keys or
// signing methods are accepted by this package; it only reads chain data.
package blockchain

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"paycore/api/internal/models"
)

type Client struct {
	paymentContract *common.Address
	client          *ethclient.Client
	chainID         uint64
	token           common.Address
	decimals        int
}

func Open(ctx context.Context, endpoint string, cfg models.QuantumConfig) (*Client, error) {
	rpc, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		return nil, errors.New("quantum RPC connection failed")
	}
	c := &Client{client: rpc, chainID: cfg.ChainID, token: common.HexToAddress(cfg.TokenAddress), decimals: cfg.TokenDecimals}
	if cfg.PaymentContractAddress != nil {
		address := common.HexToAddress(*cfg.PaymentContractAddress)
		c.paymentContract = &address
	}
	if err := c.Validate(ctx); err != nil {
		rpc.Close()
		return nil, err
	}
	return c, nil
}
func (c *Client) Close() { c.client.Close() }
func (c *Client) Validate(ctx context.Context) error {
	chain, err := c.client.ChainID(ctx)
	if err != nil {
		return errors.New("quantum RPC chain ID unavailable")
	}
	if !chain.IsUint64() || chain.Uint64() != c.chainID {
		return errors.New("quantum RPC chain ID mismatch")
	}
	code, err := c.client.CodeAt(ctx, c.token, nil)
	if err != nil || len(code) == 0 {
		return errors.New("quantum token contract unavailable")
	}
	// decimals() selector; decoded as a canonical ABI uint8, never as float money.
	result, err := c.client.CallContract(ctx, ethereum.CallMsg{To: &c.token, Data: common.FromHex("0x313ce567")}, nil)
	if err != nil || len(result) != 32 {
		return errors.New("quantum token decimals unavailable")
	}
	n := new(big.Int).SetBytes(result)
	if !n.IsInt64() || n.Int64() != int64(c.decimals) {
		return errors.New("quantum token decimals mismatch")
	}
	if c.paymentContract != nil {
		code, err := c.client.CodeAt(ctx, *c.paymentContract, nil)
		if err != nil || len(code) == 0 {
			return errors.New("quantum payment contract unavailable")
		}
		result, err := c.client.CallContract(ctx, ethereum.CallMsg{To: c.paymentContract, Data: common.FromHex("0xfc0c546a")}, nil)
		if err != nil || len(result) != 32 || new(big.Int).SetBytes(result[:12]).Sign() != 0 || common.BytesToAddress(result[12:]) != c.token {
			return errors.New("quantum payment contract token mismatch")
		}
	}
	return nil
}
func (c *Client) Verify(ctx context.Context, hash string) (models.QuantumReceipt, error) {
	var proof models.QuantumReceipt
	chain, err := c.client.ChainID(ctx)
	if err != nil {
		return proof, errors.New("quantum RPC unavailable")
	}
	if !chain.IsUint64() {
		return proof, errors.New("quantum RPC chain ID overflow")
	}
	proof.ChainID = chain.Uint64()
	if proof.ChainID != c.chainID {
		return proof, nil
	}
	receipt, err := c.client.TransactionReceipt(ctx, common.HexToHash(hash))
	if errors.Is(err, ethereum.NotFound) {
		return proof, nil
	}
	if err != nil {
		return proof, errors.New("quantum receipt unavailable")
	}
	if receipt.BlockNumber == nil || !receipt.BlockNumber.IsInt64() || receipt.BlockNumber.Sign() < 0 || receipt.TxHash != common.HexToHash(hash) {
		return proof, errors.New("invalid Quantum receipt context")
	}
	tx, pending, err := c.client.TransactionByHash(ctx, common.HexToHash(hash))
	if err != nil {
		return proof, errors.New("quantum transaction unavailable")
	}
	if pending {
		return proof, nil
	}
	if tx.Hash() != receipt.TxHash || !tx.ChainId().IsUint64() || tx.ChainId().Uint64() != c.chainID {
		return proof, errors.New("invalid Quantum transaction context")
	}
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return proof, errors.New("invalid Quantum transaction signature")
	}
	header, err := c.client.HeaderByNumber(ctx, receipt.BlockNumber)
	if err != nil {
		return proof, errors.New("quantum canonical block unavailable")
	}
	head, err := c.client.BlockNumber(ctx)
	if err != nil {
		return proof, errors.New("quantum head unavailable")
	}
	proof.Found = true
	proof.Success = receipt.Status == types.ReceiptStatusSuccessful
	proof.BlockNumber = receipt.BlockNumber.Uint64()
	proof.BlockHash = strings.ToLower(receipt.BlockHash.Hex())
	proof.Canonical = header.Hash() == receipt.BlockHash && header.Number != nil && header.Number.Cmp(receipt.BlockNumber) == 0
	if header.Time > math.MaxInt64 {
		return proof, errors.New("invalid Quantum block timestamp")
	}
	proof.BlockTime = time.Unix(int64(header.Time), 0).UTC()
	if head >= proof.BlockNumber {
		depth := head - proof.BlockNumber
		if depth >= math.MaxInt64 {
			return proof, errors.New("invalid Quantum confirmation depth")
		}
		proof.Confirmations = int64(depth) + 1
	}
	proof.TxSender = strings.ToLower(sender.Hex())
	if tx.To() != nil {
		proof.TxRecipient = strings.ToLower(tx.To().Hex())
	}
	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	for _, log := range receipt.Logs {
		if log.Removed {
			proof.Canonical = false
			continue
		}
		if log.TxHash != receipt.TxHash || log.BlockHash != receipt.BlockHash || log.BlockNumber != proof.BlockNumber {
			continue
		}
		if event, ok := decodeInvoicePaid(log); ok {
			proof.Payments = append(proof.Payments, event)
		}
		if log.Address != c.token || len(log.Topics) != 3 || log.Topics[0] != transferTopic || len(log.Data) != 32 || uint64(log.Index) > math.MaxInt64 {
			continue
		}
		// Indexed addresses must be zero-padded ABI words, not truncated arbitrary topics.
		if new(big.Int).SetBytes(log.Topics[1][:12]).Sign() != 0 || new(big.Int).SetBytes(log.Topics[2][:12]).Sign() != 0 {
			continue
		}
		amount := new(big.Int).SetBytes(log.Data)
		t := models.QuantumTransfer{LogIndex: uint64(log.Index), Token: strings.ToLower(log.Address.Hex()), From: strings.ToLower(common.BytesToAddress(log.Topics[1][12:]).Hex()), To: strings.ToLower(common.BytesToAddress(log.Topics[2][12:]).Hex()), Overflow: !amount.IsInt64()}
		if !t.Overflow {
			t.Amount = amount.Int64()
		}
		proof.Transfers = append(proof.Transfers, t)
	}
	return proof, nil
}
