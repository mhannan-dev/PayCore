import { Icon } from "../icons";

export type StatProps = { label: string; value: string; foot: string; icon: string };

export function Stat({ label, value, foot, icon }: StatProps) {
  return (
    <div className="stat-card">
      <div className="stat-label">{label}<Icon name={icon} /></div>
      <div className="stat-value">{value}</div>
      <div className="stat-foot">{foot}</div>
    </div>
  );
}

export function StatGrid({ stats, three = false }: { stats: StatProps[]; three?: boolean }) {
  return (
    <div className={three ? "stats-grid three" : "stats-grid"}>
      {stats.map((stat) => <Stat key={stat.label} {...stat} />)}
    </div>
  );
}
