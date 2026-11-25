import { isAddress, type Address, type Hash } from "viem";
import { baseUnitsToBigInt } from "./quantum-amount";
import type { Checkout } from "./quantum-api";

export const QUANTUM_TOKEN_ABI = [
  { type: "function", name: "allowance", stateMutability: "view", inputs: [{ name: "owner", type: "address" }, { name: "spender", type: "address" }], outputs: [{ type: "uint256" }] },
  { type: "function", name: "approve", stateMutability: "nonpayable", inputs: [{ name: "spender", type: "address" }, { name: "amount", type: "uint256" }], outputs: [{ type: "bool" }] },
  { type: "function", name: "transfer", stateMutability: "nonpayable", inputs: [{ name: "to", type: "address" }, { name: "amount", type: "uint256" }], outputs: [{ type: "bool" }] },
] as const;

export const QUANTUM_PAYMENTS_ABI = [
  { type: "function", name: "pay", stateMutability: "nonpayable", inputs: [{ name: "orderId", type: "bytes32" }, { name: "merchant", type: "address" }, { name: "amount", type: "uint256" }], outputs: [] },
] as const;

type Payment = { token: Address; merchant: Address; amount: bigint };
type ContractPayment = Payment & { contract: Address; orderId: Hash };

export type CheckoutPaymentActions = {
  allowance: (payment: ContractPayment) => Promise<bigint>;
  approve: (payment: ContractPayment) => Promise<Hash>;
  waitForApproval: (hash: Hash) => Promise<void>;
  register: () => Promise<string>;
  pay: (payment: ContractPayment) => Promise<Hash>;
  transfer: (payment: Payment) => Promise<Hash>;
};

export async function sendCheckoutPayment(checkout: Checkout, actions: CheckoutPaymentActions, progress: (message: string) => void): Promise<{ attemptId: string; hash: Hash }> {
  if (!isAddress(checkout.token_address, { strict: false }) || !isAddress(checkout.receiver_address, { strict: false })) {
    throw new Error("This invoice has invalid payment details. Contact the merchant.");
  }
  const payment: Payment = { token: checkout.token_address, merchant: checkout.receiver_address, amount: baseUnitsToBigInt(checkout.amount_base_units) };
  if (checkout.payment_contract_address === null) {
    const attemptId = await actions.register();
    progress("Confirm the transfer in your wallet.");
    return { attemptId, hash: await actions.transfer(payment) };
  }
  if (!isAddress(checkout.payment_contract_address, { strict: false }) || !/^0x[0-9a-f]{64}$/.test(checkout.order_id)) {
    throw new Error("This invoice has invalid contract details. Contact the merchant.");
  }
  // isAddress narrows addresses; bytes32 is checked above at the API boundary.
  const orderId: Hash = `0x${checkout.order_id.slice(2)}`;
  const contractPayment: ContractPayment = { ...payment, contract: checkout.payment_contract_address, orderId };
  if (await actions.allowance(contractPayment) < payment.amount) {
    progress("Approve this invoice amount in your wallet. Approval does not pay the invoice.");
    const hash = await actions.approve(contractPayment);
    progress("Waiting for token approval to confirm…");
    await actions.waitForApproval(hash);
    if (await actions.allowance(contractPayment) < payment.amount) {
      throw new Error("Token approval is insufficient. No payment was sent.");
    }
  }
  // Register after approval so a slow approval cannot consume the intent window.
  const attemptId = await actions.register();
  progress("Approval ready. Confirm the invoice payment in your wallet.");
  return { attemptId, hash: await actions.pay(contractPayment) };
}
