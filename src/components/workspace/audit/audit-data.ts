import { journals } from "@/lib/demo-data";

export type JournalCheck = { id: string; debit: bigint; credit: bigint; balanced: boolean };

const sum = (amounts: string[]) => amounts.reduce((s, a) => s + BigInt(a), BigInt(0));

export function journalChecks(): JournalCheck[] {
  return journals.map((j) => {
    const debit = sum(j.postings.filter((p) => p.direction === "DEBIT").map((p) => p.amount));
    const credit = sum(j.postings.filter((p) => p.direction === "CREDIT").map((p) => p.amount));
    const wellFormed = j.postings.length >= 2 && j.postings.every((p) => BigInt(p.amount) > BigInt(0));
    return { id: j.id, debit, credit, balanced: wellFormed && debit === credit };
  });
}
