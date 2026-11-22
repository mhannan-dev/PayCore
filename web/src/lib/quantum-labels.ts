import type { AttemptStatus, FailureCode, InvoiceStatus } from "./quantum-api";

export type QuantumTone = "green" | "amber" | "neutral";

export function invoiceStatusLabel(status: InvoiceStatus): string {
  switch (status) {
    case "PENDING": return "Awaiting payment";
    case "PAID": return "Paid";
    case "EXPIRED": return "Expired";
    case "CANCELLED": return "Cancelled";
  }
}

export function invoiceStatusTone(status: InvoiceStatus): QuantumTone {
  switch (status) {
    case "PENDING": return "amber";
    case "PAID": return "green";
    case "EXPIRED":
    case "CANCELLED": return "neutral";
  }
}

export function attemptStatusLabel(status: AttemptStatus): string {
  switch (status) {
    case "CREATED": return "Intent registered";
    case "BROADCAST": return "Transaction submitted";
    case "VERIFYING": return "Confirming on testnet";
    case "CONFIRMED": return "Confirmed";
    case "FAILED": return "Failed";
    case "REORGED": return "Reorganised";
    case "REVIEW_REQUIRED": return "Needs review";
  }
}

export function attemptStatusTone(status: AttemptStatus): QuantumTone {
  switch (status) {
    case "CONFIRMED": return "green";
    case "CREATED":
    case "BROADCAST":
    case "VERIFYING": return "amber";
    case "FAILED":
    case "REORGED":
    case "REVIEW_REQUIRED": return "neutral";
  }
}

export function failureCodeMessage(code: FailureCode): string {
  switch (code) {
    case "wrong_chain": return "The payment was sent on a different network. Use the test network shown on this page.";
    case "wrong_token": return "The payment used a different token. Send only the token shown on this page.";
    case "wrong_recipient": return "The payment went to a different address. Send it to the receiver shown on this page.";
    case "wrong_payer": return "The payment came from a different wallet than the one registered for this attempt.";
    case "underpaid": return "The payment was less than the invoice amount, so it cannot settle. Contact the merchant for help.";
    case "overpaid": return "The payment was more than the invoice amount, so it cannot settle. Contact the merchant for help.";
    case "late_payment": return "The transaction was mined after the invoice expired. Contact the merchant for help.";
    case "tx_failed": return "The testnet transaction failed. Nothing was paid; you may register a new attempt.";
    case "tx_not_found": return "The transaction could not be found on the testnet. Check the explorer before trying again.";
    case "duplicate_transfer": return "This transfer was already used for another invoice. Contact the merchant instead of sending again.";
  }
}
