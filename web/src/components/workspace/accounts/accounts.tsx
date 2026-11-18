import { accounts, balance, balanceOf, money } from "@/lib/demo-data";
import { Heading, StatGrid } from "../../ui";
import { AccountsTable } from "./accounts-table";

const liabilityTotal = () =>
  accounts.filter((a) => a.type === "LIABILITY").reduce((sum, a) => sum + balance(a), BigInt(0));

export function Accounts() {
  return (
    <>
      <Heading eyebrow="LEDGER CORE" title="Chart of accounts" description="The building blocks of your ledger. Select an account to inspect its postings." />
      <StatGrid
        three
        stats={[
          { label: "Asset balance", value: money(balanceOf("1001")), foot: "Operating bank · JPY", icon: "bank" },
          { label: "Liability balance", value: money(liabilityTotal()), foot: "Wallets and payout clearing", icon: "wallet" },
          { label: "Revenue balance", value: money(balanceOf("4001")), foot: "Platform fees · JPY", icon: "chart" },
        ]}
      />
      <AccountsTable />
      <p className="footnote">Account creation will be enabled when the Ledger Core account API is connected.</p>
    </>
  );
}
