-- F1/F2: retain unused intents and persist evidence across worker restarts.
ALTER TABLE quantum_payment_attempts DROP CONSTRAINT quantum_payment_attempts_failure_code_check;
ALTER TABLE quantum_payment_attempts ADD CONSTRAINT quantum_payment_attempts_failure_code_check
 CHECK(failure_code IN('wrong_chain','wrong_token','wrong_contract','wrong_order','wrong_recipient',
 'wrong_payer','underpaid','overpaid','late_payment','tx_failed','tx_not_found','duplicate_transfer','abandoned'));
ALTER TABLE quantum_payment_attempts DROP CONSTRAINT quantum_payment_attempts_check1;
ALTER TABLE quantum_payment_attempts ADD CONSTRAINT quantum_attempt_hash_state
 CHECK(status='CREATED' OR tx_hash IS NOT NULL OR (status='FAILED' AND failure_code='abandoned'));
ALTER TABLE quantum_payment_attempts
 ADD COLUMN reorg_suspicions INTEGER NOT NULL DEFAULT 0 CHECK(reorg_suspicions BETWEEN 0 AND 5),
 ADD COLUMN reorg_suspected_at TIMESTAMPTZ,
 ADD COLUMN confirmed_at TIMESTAMPTZ,
 ADD COLUMN verification_finalized BOOLEAN NOT NULL DEFAULT false;
UPDATE quantum_payment_attempts SET confirmed_at=updated_at WHERE status='CONFIRMED';
