import Money from './Money';

/**
 * The customer signature: an allowance rail with a hard tick at the included
 * boundary, and overage extending past it in amber.
 *
 * The fill NEVER changes color to signal danger — that encoding has to stay
 * stable, because it is the one thing on the page that says "this part is
 * paid for and this part is not". Pressure is carried by the caption instead:
 * warning at 80%, danger at 100%.
 */

const trackHeights = { sm: 'h-2', md: 'h-2', hero: 'h-3' };

function toDate(value) {
  if (!value) return null;
  if (value instanceof Date) return value;
  if (typeof value === 'number') return new Date(value < 1e12 ? value * 1000 : value);
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

export default function Meter({
  usedNano = 0,
  includedNano = 0,
  size = 'md',
  periodEnd,
  label,
  className = '',
}) {
  const used = Math.max(0, Number(usedNano) || 0);
  const included = Math.max(0, Number(includedNano) || 0);
  const total = Math.max(included, used);

  const pct = (n) => (total > 0 ? Math.min(100, (n / total) * 100) : 0);
  const overage = Math.max(0, used - included);

  // With no included allowance every dollar spent is charged straight to
  // prepaid credit, so the whole rail is overage. Filling it with the primary
  // "this is paid for" color would claim something untrue.
  const consumedPct = included > 0 ? pct(Math.min(used, included)) : 0;
  const overagePct = included > 0 ? pct(overage) : pct(used);
  const tickPct = included > 0 && total > 0 ? (included / total) * 100 : null;

  const ratio = included > 0 ? used / included : 0;
  const captionTone =
    included > 0 && ratio >= 1 ? 'text-danger' : included > 0 && ratio >= 0.8 ? 'text-warning' : 'text-muted';

  const end = toDate(periodEnd);
  const daysLeft = end ? Math.max(0, Math.ceil((end.getTime() - Date.now()) / 86400000)) : null;

  const valuetext =
    included > 0
      ? `${((used / included) * 100).toFixed(0)}% of included allowance used`
      : 'No included allowance';

  return (
    <div className={className}>
      {label && <p className="text-label text-text mb-2">{label}</p>}

      {/* At hero size the boundary is named, not just drawn. The label sits
          directly over the tick so the rail reads left-to-right as
          paid-for → boundary → overage. */}
      {size === 'hero' && tickPct != null && tickPct < 100 && (
        <div className="relative h-3.5" aria-hidden="true">
          <span
            className="absolute bottom-0 -translate-x-1/2 whitespace-nowrap text-micro uppercase text-muted"
            style={{ left: `${Math.min(92, Math.max(8, tickPct))}%` }}
          >
            Included
          </span>
        </div>
      )}

      <div
        role="meter"
        aria-valuenow={Math.round(used)}
        aria-valuemin={0}
        aria-valuemax={Math.round(total) || 1}
        aria-valuetext={valuetext}
        aria-label={label || 'Allowance used'}
        className={`relative w-full rounded-full bg-surface-sunken border border-border-subtle overflow-visible ${trackHeights[size] || trackHeights.md}`}
      >
        <div className="absolute inset-0 flex rounded-full overflow-hidden">
          <div
            className="h-full bg-primary motion-safe:transition-[width] motion-safe:duration-400 motion-safe:ease-out"
            style={{ width: `${consumedPct}%` }}
          />
          <div
            className="h-full bg-warning motion-safe:transition-[width] motion-safe:duration-400 motion-safe:ease-out"
            style={{ width: `${overagePct}%` }}
          />
        </div>

        {/* The hard boundary. Overhangs the rail so it reads as a machined
            stop rather than a shading change. */}
        {tickPct != null && tickPct < 100 && (
          <span
            aria-hidden="true"
            className="absolute -top-0.5 -bottom-0.5 w-0.5 bg-border-strong rounded-full"
            style={{ left: `calc(${tickPct}% - 1px)` }}
          />
        )}
      </div>

      {size !== 'sm' && (
        <div className="mt-2 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <span className={`text-caption ${captionTone}`}>
            <Money nano={used} size="label" className="font-medium" />
            {included > 0 ? (
              <span className="text-muted"> of <Money nano={included} size="label" /> included</span>
            ) : (
              <span className="text-muted"> used this period</span>
            )}
          </span>

          {included > 0 && overage > 0 && (
            <span className="text-caption text-warning">
              <Money nano={overage} size="label" className="font-medium" /> overage
            </span>
          )}

          {size === 'hero' && end && (
            <span className="text-caption text-muted">
              Renews {end.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
              {daysLeft != null && ` · ${daysLeft} day${daysLeft === 1 ? '' : 's'} left`}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export { Meter };
