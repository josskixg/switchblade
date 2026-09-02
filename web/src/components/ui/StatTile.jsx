import { ArrowDownRight, ArrowUpRight } from 'lucide-react';
import Money from './Money';
import Skeleton from './Skeleton';
import Sparkline from './Sparkline';

/**
 * No colored icon tile. The four Overview tiles used to carry lime / purple /
 * blue / red icons, implying a categorical encoding that does not exist — the
 * metric's size and weight carry the emphasis instead.
 *
 * `delta` is colored by DIRECTION OF DESIRABILITY, not by sign: falling cost
 * is good news. Pass deltaDirection="down-is-good" for cost and latency.
 */
export default function StatTile({
  label,
  value,
  unit,
  nano,
  size = 'md',
  delta,
  deltaDirection = 'up-is-good',
  sparkline,
  sparklineSeries = 0,
  icon: Icon,
  loading = false,
  empty = false,
  emptyLabel = 'No data yet',
  className = '',
}) {
  const isMoney = nano != null;
  const hero = size === 'hero';

  const deltaNum = typeof delta === 'number' ? delta : null;
  const good =
    deltaNum == null || deltaNum === 0
      ? null
      : deltaDirection === 'down-is-good'
        ? deltaNum < 0
        : deltaNum > 0;
  const DeltaIcon = deltaNum != null && deltaNum < 0 ? ArrowDownRight : ArrowUpRight;

  return (
    <div className={`rounded-lg border border-border bg-surface shadow-1 p-5 ${className}`}>
      <div className="flex items-start justify-between gap-3">
        <p className="text-label text-muted">{label}</p>
        {Icon && <Icon className="w-4 h-4 text-muted shrink-0" aria-hidden="true" />}
      </div>

      <div className="mt-2 min-h-[2rem] flex items-end justify-between gap-3">
        {loading ? (
          <Skeleton className="h-7 w-24" />
        ) : empty ? (
          // Never render 0 when the data failed to arrive.
          <span className="text-body text-muted">{emptyLabel}</span>
        ) : (
          <span className="flex items-baseline gap-1.5 min-w-0">
            {isMoney ? (
              <Money nano={nano} size={hero ? 'hero' : 'metric'} />
            ) : (
              <span className={`${hero ? 'text-display-2' : 'text-metric-md'} text-text tabular-nums lining-nums truncate`}>
                {value}
              </span>
            )}
            {unit && <span className="text-caption text-muted shrink-0">{unit}</span>}
          </span>
        )}

        {!loading && !empty && sparkline && (
          <Sparkline data={sparkline} series={sparklineSeries} className="shrink-0 opacity-90" />
        )}
      </div>

      {!loading && !empty && deltaNum != null && (
        <p
          className={`mt-2 inline-flex items-center gap-1 text-caption tabular-nums lining-nums ${
            good === null ? 'text-muted' : good ? 'text-success' : 'text-danger'
          }`}
        >
          <DeltaIcon className="w-3.5 h-3.5" aria-hidden="true" />
          {Math.abs(deltaNum).toLocaleString('en-US', { maximumFractionDigits: 1 })}%
          <span className="text-muted">vs. previous period</span>
        </p>
      )}
    </div>
  );
}

export { StatTile };
