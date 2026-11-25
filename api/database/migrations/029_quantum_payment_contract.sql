-- Existing invoices retain direct-transfer semantics; new invoices snapshot config.
ALTER TABLE quantum_invoices ADD COLUMN payment_contract_address TEXT
 CHECK (payment_contract_address IS NULL OR (
   payment_contract_address ~ '^0x[0-9a-f]{40}$'
   AND payment_contract_address <> '0x0000000000000000000000000000000000000000'
   AND payment_contract_address <> token_address
 ));

ALTER TABLE quantum_payment_attempts DROP CONSTRAINT quantum_payment_attempts_failure_code_check;
ALTER TABLE quantum_payment_attempts ADD CONSTRAINT quantum_payment_attempts_failure_code_check
 CHECK(failure_code IN('wrong_chain','wrong_token','wrong_contract','wrong_order','wrong_recipient',
 'wrong_payer','underpaid','overpaid','late_payment','tx_failed','tx_not_found','duplicate_transfer'));
