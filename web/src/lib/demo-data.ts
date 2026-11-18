export type Account = { id: string; name: string; type: "ASSET" | "LIABILITY" | "REVENUE"; currency: "JPY" };
export type Posting = { accountId: string; direction: "DEBIT" | "CREDIT"; amount: string };
export type Journal = { id: string; description: string; time: string; category: string; postings: Posting[] };

export const accounts: Account[] = [
  { id: "1001", name: "Operating bank", type: "ASSET", currency: "JPY" },
  { id: "2001", name: "Akira · Wallet", type: "LIABILITY", currency: "JPY" },
  { id: "2002", name: "Hana · Wallet", type: "LIABILITY", currency: "JPY" },
  { id: "2003", name: "Payout clearing", type: "LIABILITY", currency: "JPY" },
  { id: "4001", name: "Platform fee revenue", type: "REVENUE", currency: "JPY" },
];
const pair = (debit: string, credit: string, amount: string): Posting[] => [
  { accountId: debit, direction: "DEBIT", amount }, { accountId: credit, direction: "CREDIT", amount },
];
// Balanced fixtures, shared by all preview pages. No real financial effects.
export const journals: Journal[] = [
  { id: "JRN-0006", description: "Sandbox payout captured", time: "14:42", category: "Settlement", postings: pair("2003", "1001", "25000") },
  { id: "JRN-0005", description: "Reserve funds for payout", time: "14:40", category: "Settlement", postings: pair("2002", "2003", "25000") },
  { id: "JRN-0004", description: "Wallet transfer platform fee", time: "13:25", category: "Fee", postings: pair("2002", "4001", "500") },
  { id: "JRN-0003", description: "Akira → Hana wallet transfer", time: "13:24", category: "Transfer", postings: pair("2001", "2002", "50000") },
  { id: "JRN-0002", description: "Hana wallet funding", time: "10:15", category: "Funding", postings: pair("1001", "2002", "300000") },
  { id: "JRN-0001", description: "Akira wallet funding", time: "09:10", category: "Funding", postings: pair("1001", "2001", "500000") },
];
export function total(direction: Posting["direction"]) {
  return journals.flatMap((j) => j.postings).reduce((sum, p) => p.direction === direction ? sum + BigInt(p.amount) : sum, BigInt(0));
}
export function balance(account: Account) {
  return journals.flatMap((j) => j.postings).filter((p) => p.accountId === account.id).reduce((sum, p) => sum + (p.direction === (account.type === "ASSET" ? "DEBIT" : "CREDIT") ? BigInt(p.amount) : -BigInt(p.amount)), BigInt(0));
}
/** Balance of the account with this ID (0 for an unknown ID). Prefer this to indexing `accounts`. */
export function balanceOf(id: string) {
  const account = accounts.find((a) => a.id === id);
  return account ? balance(account) : BigInt(0);
}
export function money(value: bigint | string) { return `¥${BigInt(value).toLocaleString("en-US")}`; }
export function accountName(id: string) { return accounts.find((a) => a.id === id)?.name ?? id; }
export function validAmount(value: string) { return /^[1-9]\d*$/.test(value) && value.length <= 19 && BigInt(value) <= BigInt("9223372036854775807"); }
