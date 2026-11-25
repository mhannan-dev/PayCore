package blockchain

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"paycore/api/internal/models"
)

func TestInvoicePaidDecoding(t *testing.T) {
	newLog := func() *types.Log {
		return &types.Log{
			Address: common.HexToAddress("0xdddddddddddddddddddddddddddddddddddddddd"), Index: 4,
			Topics: []common.Hash{crypto.Keccak256Hash([]byte("InvoicePaid(bytes32,address,address,uint256,bytes32)")), common.HexToHash(models.QuantumOrderID("11111111-1111-4111-8111-111111111111")), common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")},
			Data:   append(common.LeftPadBytes(big.NewInt(1250000).Bytes(), 32), make([]byte, 32)...),
		}
	}
	for _, tc := range []struct {
		name  string
		alter func(*types.Log)
	}{
		{"removed", func(l *types.Log) { l.Removed = true }},
		{"short topics", func(l *types.Log) { l.Topics = l.Topics[:3] }},
		{"wrong signature", func(l *types.Log) { l.Topics[0] = common.Hash{} }},
		{"short data", func(l *types.Log) { l.Data = l.Data[:32] }},
		{"trailing data", func(l *types.Log) { l.Data = append(l.Data, 0) }},
		{"payer padding", func(l *types.Log) { l.Topics[2][0] = 1 }},
		{"merchant padding", func(l *types.Log) { l.Topics[3][0] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newLog()
			tc.alter(l)
			if _, ok := decodeInvoicePaid(l); ok {
				t.Fatal("malformed event accepted")
			}
		})
	}
	l := newLog()
	event, ok := decodeInvoicePaid(l)
	if !ok || event.Amount != 1250000 || event.LogIndex != 4 || event.Overflow || event.Payer != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("event: %+v", event)
	}
	l.Data[0] = 0xff
	if event, ok := decodeInvoicePaid(l); !ok || !event.Overflow {
		t.Fatal("overflow silently truncated")
	}
}

func TestPaymentContractStartup(t *testing.T) {
	const token = "0xcccccccccccccccccccccccccccccccccccccccc"
	const contract = "0xdddddddddddddddddddddddddddddddddddddddd"
	for _, scenario := range []string{"valid", "missing code", "wrong token", "malformed token"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     json.RawMessage   `json:"id"`
					Method string            `json:"method"`
					Params []json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					return
				}
				result := "0x3039"
				switch req.Method {
				case "eth_getCode":
					result = "0x6000"
					if scenario == "missing code" && strings.Contains(string(req.Params[0]), contract) {
						result = "0x"
					}
				case "eth_call":
					result = "0x" + strings.Repeat("0", 63) + "6"
					if strings.Contains(string(req.Params[0]), contract) {
						result = "0x" + strings.Repeat("0", 24) + token[2:]
						if scenario == "wrong token" {
							result = "0x" + strings.Repeat("0", 64)
						}
						if scenario == "malformed token" {
							result = "0x01"
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(struct {
					Version string          `json:"jsonrpc"`
					ID      json.RawMessage `json:"id"`
					Result  string          `json:"result"`
				}{"2.0", req.ID, result})
			}))
			defer server.Close()
			address := contract
			client, err := Open(context.Background(), server.URL, models.QuantumConfig{ChainID: 12345, TokenAddress: token, TokenDecimals: 6, PaymentContractAddress: &address})
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("startup: %v", err)
			}
			if client != nil {
				client.Close()
			}
		})
	}
}
