import { money } from "@/lib/demo-data";

const HOURS = ["09:00", "10:00", "11:00", "12:00", "13:00", "14:00"];
const VALUES = [500000, 300000, 0, 0, 50500, 50000];

export function ActivityChart() {
  return (
    <div
      className="activity-chart"
      role="img"
      aria-label="Sample hourly debit volume: 09:00 500000 yen, 10:00 300000, 11:00 and 12:00 zero, 13:00 50500, 14:00 50000."
    >
      <div className="chart-y"><span>500k</span><span>250k</span><span>0</span></div>
      <div className="chart-bars">
        {VALUES.map((value, i) => (
          <div className="chart-column" key={HOURS[i]}>
            <div className="bar-space">
              <div className="chart-bar" style={{ height: `${Math.max(1, value / 5000)}%` }}>
                <span>{money(BigInt(value))}</span>
              </div>
            </div>
            <span>{HOURS[i]}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
