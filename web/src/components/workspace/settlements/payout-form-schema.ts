import { object, string, type InferType } from "yup";
import type { CreatePayoutRequest, Currency, LedgerAccount } from "@/lib/ledger-api";
import { parseMoneyInput } from "@/lib/money";

// Mirrors CreatePayoutRequest.Normalize; amounts entered here are major units.
export const payoutSchema = (currency: Currency) => object({
  source_account_id: string().required("Choose a source wallet.").matches(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i, "Choose a valid wallet."),
  amount: string().trim().required("Enter an amount.").test("money", "Enter a positive amount within this currency's decimal precision and int64 limits.", (value) => typeof value === "string" && !("error" in parseMoneyInput(value, currency))),
  beneficiary_name: string().trim().required("Enter the beneficiary name.").max(120),
  beneficiary_bank: string().trim().required("Enter the beneficiary bank.").max(120),
  beneficiary_account: string().trim().required("Enter the beneficiary account.").max(120),
});
export type PayoutValues = InferType<ReturnType<typeof payoutSchema>>;

export function buildPayoutRequest(values: PayoutValues, currency: Currency): CreatePayoutRequest {
  const validated = payoutSchema(currency).validateSync(values);
  const amount = parseMoneyInput(validated.amount, currency);
  if ("error" in amount) throw new Error(amount.error);
  return { ...validated, currency, amount: amount.minor };
}

export function eligiblePayoutSource(account: LedgerAccount, currency: Currency): boolean {
  return account.is_active && account.currency === currency && account.type === "LIABILITY"
    && account.account_number !== "2003" && !account.name.toLowerCase().includes("clearing");
}
