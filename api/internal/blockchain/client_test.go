package blockchain

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"paycore/api/internal/models"
)

func TestQuantumRPCStartupChecks(t *testing.T) {
	for _, tc := range []struct {
		name, chain, code, decimals string
		ok                          bool
	}{{"valid", "0x3039", "0x6000", "06", true}, {"wrong chain", "0x1", "0x6000", "06", false}, {"missing contract", "0x3039", "0x", "06", false}, {"wrong decimals", "0x3039", "0x6000", "07", false}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				value := tc.chain
				if req.Method == "eth_getCode" {
					value = tc.code
				}
				if req.Method == "eth_call" {
					value = "0x" + strings.Repeat("0", 62) + tc.decimals
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(struct {
					Version string          `json:"jsonrpc"`
					ID      json.RawMessage `json:"id"`
					Result  string          `json:"result"`
				}{"2.0", req.ID, value})
			}))
			defer server.Close()
			client, err := Open(context.Background(), server.URL, models.QuantumConfig{ChainID: 12345, TokenAddress: "0xcccccccccccccccccccccccccccccccccccccccc", TokenDecimals: 6})
			if (err == nil) != tc.ok {
				t.Fatalf("startup: %v", err)
			}
			if client != nil {
				client.Close()
			}
		})
	}
}
func TestQuantumRPCReceiptContext(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	token := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")
	receiver := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	tx, err := types.SignTx(types.NewTx(&types.LegacyTx{Nonce: 0, To: &token, Gas: 60000, GasPrice: big.NewInt(1)}), types.LatestSignerForChainID(big.NewInt(12345)), key)
	if err != nil {
		t.Fatal(err)
	}
	sender := crypto.PubkeyToAddress(key.PublicKey)
	header := &types.Header{Number: big.NewInt(20), Difficulty: big.NewInt(1), GasLimit: 100000, Time: uint64(time.Now().Unix())}
	topic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	transfer := &types.Log{Address: token, Topics: []common.Hash{topic, common.BytesToHash(sender.Bytes()), common.BytesToHash(receiver.Bytes())}, Data: common.LeftPadBytes(big.NewInt(1250000).Bytes(), 32), BlockNumber: 20, TxHash: tx.Hash(), BlockHash: header.Hash()}
	receipt := &types.Receipt{Type: 0, Status: 1, CumulativeGasUsed: 21000, Logs: []*types.Log{transfer}, TxHash: tx.Hash(), BlockHash: header.Hash(), BlockNumber: big.NewInt(20), GasUsed: 21000, EffectiveGasPrice: big.NewInt(1)}
	raw, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	var txObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &txObject); err != nil {
		t.Fatal(err)
	}
	txObject["blockNumber"] = json.RawMessage(`"0x14"`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch req.Method {
		case "eth_chainId":
			result = "0x3039"
		case "eth_getCode":
			result = "0x6000"
		case "eth_call":
			result = "0x" + strings.Repeat("0", 62) + "06"
		case "eth_getTransactionReceipt":
			result = receipt
		case "eth_getTransactionByHash":
			result = txObject
		case "eth_getBlockByNumber":
			result = header
		case "eth_blockNumber":
			result = "0x15"
		default:
			t.Errorf("unexpected RPC method %s", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Version string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  any             `json:"result"`
		}{"2.0", req.ID, result}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := Open(context.Background(), server.URL, models.QuantumConfig{ChainID: 12345, TokenAddress: strings.ToLower(token.Hex()), TokenDecimals: 6})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	proof, err := client.Verify(context.Background(), strings.ToLower(tx.Hash().Hex()))
	if err != nil {
		t.Fatal(err)
	}
	if !proof.Found || !proof.Canonical || proof.Confirmations != 2 || proof.TxSender != strings.ToLower(sender.Hex()) || len(proof.Transfers) != 1 || proof.Transfers[0].Amount != 1250000 {
		t.Fatalf("proof: %+v", proof)
	}
}
