/**
 * Superseded by Meter. Kept for the pages that still import it, but rebuilt on
 * semantic tokens — it used to inject raw Tailwind 400-shades (1.67:1 on
 * white) and carry its own duplicate tier→color map.
 *
 * Same encoding as Meter: the fill does not change color to signal danger,
 * the caption does.
 */
export default function QuotaBar({ used = 0, total = 1, label, className = '' }) {
  const pct = total > 0 ? Math.min((used / total) * 100, 100) : 0;
  const ratio = total > 0 ? used / total : 0;
  const captionTone = ratio >= 1 ? 'text-danger' : ratio >= 0.8 ? 'text-warning' : 'text-muted';

  return (
    <div className={`space-y-1.5 ${className}`}>
      {label && (
        <div className="flex items-center justify-between gap-3">
          <span className="text-caption text-muted">{label}</span>
          <span className={`text-caption font-mono tabular-nums lining-nums ${captionTone}`}>
            {Number(used).toLocaleString('en-US')} / {Number(total).toLocaleString('en-US')}
          </span>
        </div>
      )}
      <div
        role="meter"
        aria-valuenow={Math.round(used)}
        aria-valuemin={0}
        aria-valuemax={Math.round(total) || 1}
        aria-label={label || 'Quota used'}
        className="h-2 rounded-full bg-surface-sunken border border-border-subtle overflow-hidden"
      >
        <div
          className="h-full bg-primary motion-safe:transition-[width] motion-safe:duration-400 motion-safe:ease-out"
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

export { QuotaBar };
