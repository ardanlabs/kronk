import { formatBytes } from '../lib/format';
import { ParamTooltip } from './ParamTooltips';

interface Props {
  // Display label on the left edge of the bar (e.g. "CUDA0", "System RAM").
  label: string;

  // Optional secondary label rendered after the primary label (e.g. device type).
  sublabel?: string;

  // Bytes currently reserved against the budget.
  used: number;

  // Bytes the resman is willing to hand out (Total × BudgetPercent − Headroom).
  budget: number;

  // Physical bytes the device reports. The bar is scaled to this value so the
  // gap between budget and total is visible as an "off-limits" tail.
  total: number;
}

// UsageBar renders a horizontal stacked bar showing:
//
//   [ used segment | budget remaining | off-budget tail ]
//                                  ▲
//                              budget cap marker
//
// Width is scaled to `total`. The fill segment is colored by saturation
// against `budget`: green < 60%, amber 60–85%, red > 85%. A vertical tick at
// `budget / total` marks the configured cap; the dimmed tail to its right is
// physical memory the resman intentionally won't allocate (BudgetPercent +
// HeadroomBytes).
export default function UsageBar({ label, sublabel, used, budget, total }: Props) {
  const safeTotal = Math.max(total, 1);
  const safeBudget = Math.max(budget, 0);
  const safeUsed = Math.max(used, 0);

  const usedPctOfTotal = (safeUsed / safeTotal) * 100;
  const budgetPctOfTotal = (safeBudget / safeTotal) * 100;
  const usedPctOfBudget = safeBudget > 0 ? (safeUsed / safeBudget) * 100 : 0;

  let fillClass = 'usage-bar-fill usage-bar-fill-low';
  if (usedPctOfBudget >= 85) {
    fillClass = 'usage-bar-fill usage-bar-fill-high';
  } else if (usedPctOfBudget >= 60) {
    fillClass = 'usage-bar-fill usage-bar-fill-mid';
  }

  return (
    <div className="usage-bar-row">
      <div className="usage-bar-label">
        <strong>{label}</strong>
        {sublabel && <span className="usage-bar-sublabel"> {sublabel}</span>}
      </div>

      <div
        className="usage-bar-track"
        title={`Used ${formatBytes(safeUsed)} / Budget ${formatBytes(safeBudget)} / Total ${formatBytes(total)}`}
      >
        <div
          className={fillClass}
          style={{ width: `${Math.min(usedPctOfTotal, 100)}%` }}
        />
        {budgetPctOfTotal < 100 && (
          <div
            className="usage-bar-cap-marker"
            style={{ left: `${Math.min(budgetPctOfTotal, 100)}%` }}
          />
        )}
      </div>

      <div className="usage-bar-readout">
        <span className="usage-bar-used">{formatBytes(safeUsed)}</span>
        <span className="usage-bar-sep"> / </span>
        <span className="usage-bar-budget">{formatBytes(safeBudget)}</span>
        <span className="usage-bar-pct"> ({usedPctOfBudget.toFixed(1)}%)</span>
        {' '}
        <ParamTooltip tooltipKey="budgetUsageBar" />
      </div>
    </div>
  );
}
