package config

import "testing"

func TestQuantumConfiguration(t *testing.T) {
	env := map[string]string{"QUANTUM_ENABLED": "true", "QUANTUM_CHAIN_ID": "12345", "QUANTUM_TOKEN_DECIMALS": "6", "QUANTUM_MIN_CONFIRMATIONS": "2", "QUANTUM_TOKEN_ADDRESS": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "QUANTUM_RPC_URL": "https://rpc.example.test", "QUANTUM_EXPLORER_URL": "https://explorer.example.test", "QUANTUM_CHECKOUT_BASE_URL": "http://localhost:3000"}
	get := func(key string) string { return env[key] }
	c, err := ParseQuantum(get)
	if err != nil || !c.Public.Enabled || c.Public.ChainID != 12345 {
		t.Fatalf("config: %+v %v", c, err)
	}
	for _, chain := range []string{"0", "1", "10", "56", "137", "8453", "42161", "44002", "bogus"} {
		env["QUANTUM_CHAIN_ID"] = chain
		if _, err := ParseQuantum(get); err == nil {
			t.Fatalf("chain %s accepted", chain)
		}
	}
	env["QUANTUM_CHAIN_ID"] = "12345"
	for _, key := range []string{"QUANTUM_TOKEN_ADDRESS", "QUANTUM_TOKEN_DECIMALS", "QUANTUM_MIN_CONFIRMATIONS", "QUANTUM_RPC_URL"} {
		old := env[key]
		env[key] = "invalid"
		if _, err := ParseQuantum(get); err == nil {
			t.Fatal("bad " + key + " accepted")
		}
		env[key] = old
	}
	env["QUANTUM_RPC_URL"] = "https://user:password@rpc.example.test"
	if _, err := ParseQuantum(get); err == nil {
		t.Fatal("credential URL accepted")
	}
	env["QUANTUM_ENABLED"] = "false"
	c, err = ParseQuantum(get)
	if err != nil || c.Public.Enabled || c.Public.ChainID != 0 {
		t.Fatal("disabled configuration is not isolated")
	}
}
