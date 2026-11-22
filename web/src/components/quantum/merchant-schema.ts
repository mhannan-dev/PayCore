import { object, string, type InferType } from "yup";
import type { CreateMerchantInput } from "@/lib/quantum-api";

export const merchantSchema = object({
  display_name: string().trim().required("Enter a display name.")
    .test("max-length", "Use at most 120 characters.", (value) => Array.from(value ?? "").length <= 120),
  receiver_address: string().trim().required("Enter a receiver address.")
    .matches(/^0x[0-9a-fA-F]{40}$/, "Enter a 0x address with 40 hexadecimal characters.")
    .test("nonzero", "The zero address cannot receive payments.", (value) => value !== "0x0000000000000000000000000000000000000000"),
});

export type MerchantValues = InferType<typeof merchantSchema>;

export function buildMerchantRequest(values: MerchantValues): CreateMerchantInput {
  const validated = merchantSchema.validateSync(values);
  return { display_name: validated.display_name, receiver_address: validated.receiver_address.toLowerCase() };
}
