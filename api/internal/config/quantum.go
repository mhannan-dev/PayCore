package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"paycore/api/internal/models"
)

type QuantumConfig struct {
	Public          models.QuantumConfig
	RPCURL          string
	CheckoutBaseURL string
}

// ParseQuantum never logs URLs or credentials. Invalid configuration disables this feature only.
func ParseQuantum(get func(string) string) (QuantumConfig, error) {
	var c QuantumConfig
	enabled := strings.TrimSpace(get("QUANTUM_ENABLED"))
	if enabled == "" || enabled == "false" {
		return c, nil
	}
	on, err := strconv.ParseBool(enabled)
	if err != nil {
		return c, errors.New("QUANTUM_ENABLED must be a boolean")
	}
	if !on {
		return c, nil
	}
	chain, err := strconv.ParseUint(get("QUANTUM_CHAIN_ID"), 10, 53)
	if err != nil || chain == 0 {
		return c, errors.New("invalid QUANTUM_CHAIN_ID")
	}
	switch chain {
	case 1, 10, 56, 137, 8453, 42161, 44002:
		return c, errors.New("mainnet chain IDs are forbidden")
	}
	decimals, err := strconv.Atoi(get("QUANTUM_TOKEN_DECIMALS"))
	if err != nil || decimals < 0 || decimals > 18 {
		return c, errors.New("invalid QUANTUM_TOKEN_DECIMALS")
	}
	confirmations, err := strconv.ParseInt(get("QUANTUM_MIN_CONFIRMATIONS"), 10, 64)
	if err != nil || confirmations < 1 || confirmations > 1000 {
		return c, errors.New("invalid QUANTUM_MIN_CONFIRMATIONS")
	}
	token := strings.ToLower(strings.TrimSpace(get("QUANTUM_TOKEN_ADDRESS")))
	if !models.ValidQuantumAddress(token) {
		return c, errors.New("invalid QUANTUM_TOKEN_ADDRESS")
	}
	rpc := strings.TrimSpace(get("QUANTUM_RPC_URL"))
	explorer := strings.TrimRight(strings.TrimSpace(get("QUANTUM_EXPLORER_URL")), "/")
	checkout := strings.TrimRight(strings.TrimSpace(get("QUANTUM_CHECKOUT_BASE_URL")), "/")
	for _, raw := range []string{rpc, explorer, checkout} {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return c, errors.New("quantum URLs must be HTTP(S) origins without credentials, queries or fragments")
		}
	}
	c = QuantumConfig{Public: models.QuantumConfig{Enabled: true, ChainID: chain, TokenAddress: token, TokenSymbol: "tUSD8", TokenDecimals: decimals, ExplorerURL: explorer, MinConfirmations: confirmations}, RPCURL: rpc, CheckoutBaseURL: checkout}
	return c, nil
}
