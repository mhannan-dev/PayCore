import { number, object, string, type InferType } from "yup";
import { parseTokenAmount } from "@/lib/quantum-amount";
import type { CreateInvoiceInput } from "@/lib/quantum-api";

export const invoiceSchema = (decimals: number) => object({
  amount: string().trim().required("Enter an amount.").test("token-amount", "Enter a positive amount with at most the token's decimal precision.", (value) => {
    if (typeof value !== "string") return false;
    try {
      parseTokenAmount(value, decimals);
      return true;
    } catch {
      return false;
    }
  }),
  description: string().trim().required("Enter a description.")
    .test("max-length", "Use at most 500 characters.", (value) => Array.from(value ?? "").length <= 500),
  expires_in_minutes: number().typeError("Enter a number of minutes from 5 to 1440.")
    .required("Enter an expiry.").integer("Use whole minutes.").min(5, "At least 5 minutes.").max(1440, "At most 1440 minutes."),
});

export type InvoiceValues = InferType<ReturnType<typeof invoiceSchema>>;

export function buildInvoiceRequest(values: InvoiceValues, decimals: number): CreateInvoiceInput {
  const validated = invoiceSchema(decimals).validateSync(values);
  return {
    amount_base_units: parseTokenAmount(validated.amount, decimals).toString(),
    description: validated.description,
    expires_in_minutes: validated.expires_in_minutes,
  };
}
