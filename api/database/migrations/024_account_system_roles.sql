-- Payouts used to find their clearing and bank accounts by number (2003/1001) or
-- by a name containing "clearing"/"bank", so renaming an account could redirect
-- money. An explicit role makes the choice deliberate and database-enforced.
ALTER TABLE accounts ADD COLUMN system_role TEXT
    CHECK (system_role IN ('PAYOUT_CLEARING', 'OPERATING_BANK'));

-- A role fixes the account's side of the balance sheet.
ALTER TABLE accounts ADD CONSTRAINT accounts_system_role_type_check CHECK (
    system_role IS NULL
    OR (system_role = 'PAYOUT_CLEARING' AND type = 'LIABILITY')
    OR (system_role = 'OPERATING_BANK' AND type = 'ASSET'));

-- At most one account per role and currency.
CREATE UNIQUE INDEX accounts_currency_system_role_key
    ON accounts (currency, system_role) WHERE system_role IS NOT NULL;

-- Keep existing databases working: give each role to the account the old
-- heuristic selected (first active match by account number, per currency).
UPDATE accounts a SET system_role = 'PAYOUT_CLEARING'
FROM (
    SELECT DISTINCT ON (currency) id FROM accounts
    WHERE is_active AND type = 'LIABILITY'
      AND (account_number = '2003' OR lower(name) LIKE '%clearing%')
    ORDER BY currency, account_number
) pick
WHERE a.id = pick.id;

UPDATE accounts a SET system_role = 'OPERATING_BANK'
FROM (
    SELECT DISTINCT ON (currency) id FROM accounts
    WHERE is_active AND type = 'ASSET'
      AND (account_number = '1001' OR lower(name) LIKE '%bank%')
    ORDER BY currency, account_number
) pick
WHERE a.id = pick.id;
