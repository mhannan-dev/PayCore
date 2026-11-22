# Quantum demo script (3–5 minutes)

This is a **testnet-only** walkthrough. tUSD8 has no real value; never use a mainnet wallet or real funds. Before starting, reverify the chain, token and faucet details linked from [API.md](API.md#configuration-and-readiness). The example values in `api/.env.example` are configuration, not constants.

## Preconditions (do these before the clock starts)

- PayCore API, web and PostgreSQL are running; `QUANTUM_ENABLED=true` and the chain worker is healthy.
- The seeded operator has `quantum:read` and `quantum:write`; `/quantum` is visible in the sidebar.
- A browser wallet has QDay Aevum testnet gas and tUSD8. Fund it through the [official QDay faucet](https://wiki.qday.info/guide/start/faucet) only.
- Use two browser profiles or windows: one signed in as the merchant, one signed out for the customer checkout.
- Do not put the checkout token in chat, screenshots or analytics; it is a payment capability.

## Beat 1 — Merchant login (20 s)

1. Open `http://localhost:3000` in the operator profile and sign in.
2. Point to **Quantum Payments** in the sidebar and open `/quantum`.
3. Say: *"This is a normal PayCore session: RBAC gates the page, and the API contract is mirrored by hand in the web client."*

Expected: the **TESTNET ONLY · tUSD8 has no real value** banner is visible above the console.

## Beat 2 — Merchant profile and invoice (50 s)

1. If `/merchants/me` returns 404, complete **Merchant setup** with a display name and the testnet receiver address.
2. Click **New invoice**. Enter `2.5`, description `Demo design retainer`, and keep the default expiry.
3. Show the inline hint that `2.5` becomes whole base units before it is sent; no float conversion occurs.
4. Submit and copy the checkout link from the invoice detail panel.

Expected: a `PENDING` invoice appears in the list with status filter and pagination. The detail panel shows amount, status, attempts and the copy-link action.

The creation form opens in a keyboard-accessible dialog. The merchant list and open detail refresh every ten seconds so confirmed payments appear without reloading. Accounts with read permission only can inspect invoices but are not offered merchant setup, invoice creation or cancellation. If cancellation races a payment, the console displays the API error and refreshes the invoice state.

Narration: *"The form accepts a human decimal, but the request carries an int64 base-unit digit string. The idempotency key is generated once per submission and reused on retry."*

## Beat 3 — Checkout on a phone-sized window (40 s)

1. Open the copied link in the signed-out profile.
2. Open DevTools and switch to a phone-sized viewport; show the single-column layout and labelled controls.
3. Point to the testnet banner, amount, receiver, chain and expiry from `GET /checkout/:token`.
4. Connect the wallet. If the wallet is on the wrong network, click **Switch network** and confirm.

Expected: no login is required; the public route is outside the authenticated app shell. Status changes are announced through `aria-live`.

## Beat 4 — Pay (60 s)

1. Click **Pay 2.5 tUSD8**.
2. The app registers the attempt, then asks the wallet to call `transfer(receiver, amount)`.
3. Approve in the wallet and show the explorer link appearing once the hash has been submitted, before any success state.
4. Point to the confirmations counter while `/status` is polled every three seconds.
5. Do not celebrate yet. Say: *"A hash only means broadcast. Success comes from the backend after the worker verifies the receipt."*

Expected: the pay button is disabled while requests are in flight, preventing double submission.

## Beat 5 — PAID and explorer link (30 s)

1. Wait for the status badge to become **PAID**.
2. Open the explorer link from the checkout page and show the transaction on the configured testnet explorer.
3. Reload the checkout page; it still renders the paid state from the API.
4. Switch back to the merchant console, refresh the invoice detail and show the `CONFIRMED` attempt plus paid transaction hash.

Expected: the attempts table links as `{explorer_url}/tx/{hash}`, and the invoice status is `PAID`.

## Beat 6 — Rejected edge case: wrong amount (60 s)

1. In the merchant console, create a second invoice for `5` tUSD8 and open its checkout link.
2. Click **Pay 5 tUSD8**, then change the amount to `1` tUSD8 in the wallet confirmation if that wallet allows editing the ERC-20 transfer. The app still submits the returned hash for the registered attempt.
3. If the wallet does not allow editing, use the public attempt and hash-submission examples in [API.md](API.md#examples-every-endpoint) with a manually sent `1` tUSD8 transfer, then reload the checkout link to observe the result. The checkout UI itself does not accept an arbitrary pasted hash.
4. Show the terminal **REVIEW_REQUIRED** state and the message to contact the merchant. The invoice remains `PENDING`.
5. Say: *"Underpayment never credits the invoice. P0 has no automatic refund, so do not send a second transfer; the merchant reviews it."*

Expected: the attempt has an `underpaid` failure code and no explorer-driven success screen. This is testnet-only; still avoid sending unnecessary transfers.

## Closing line (10 s)

*"PayCore Quantum demonstrates a noncustodial testnet checkout with explicit state, exact integer amounts, backend-authoritative confirmation and a clear review path for chain mismatches."*

See [ARCHITECTURE.md](ARCHITECTURE.md) for the flow diagrams and [AGENT_MANDATE.md](AGENT_MANDATE.md) for the future agent-spending design proposal.
