import Link from "next/link";
import { Icon } from "../../icons";

const LINKS = [
  { href: "/accounts", icon: "wallet", title: "Know your accounts", text: "Explore accounts and balances." },
  { href: "/simulator", icon: "code", title: "Try a safer retry", text: "See idempotency in action." },
  { href: "/settlements", icon: "bank", title: "Follow a payout", text: "Inspect the settlement lifecycle." },
];

export function QuickLinks() {
  return (
    <div className="quick-grid">
      {LINKS.map((item) => (
        <Link key={item.href} href={item.href} className="quick-card">
          <span className="quick-icon"><Icon name={item.icon} /></span>
          <div><h3>{item.title}</h3><p>{item.text}</p></div>
          <Icon name="arrow" size={17} />
        </Link>
      ))}
    </div>
  );
}
