import SegmentedControl from './SegmentedControl';
import EmptyState from './EmptyState';
import ErrorState from './ErrorState';
import Skeleton from './Skeleton';
import { useChartTheme } from '../../theme/ThemeContext';

/**
 * The card, title, period control, legend and empty/error state around a
 * chart — so a chart is never asked to invent data to fill its frame.
 *
 * Children may be a node or a render function receiving the resolved chart
 * theme: {(theme) => <BarChart …/>}.
 *
 * legend: [{ label, series }] — series indexes into the theme palette.
 */
export default function ChartFrame({
  title,
  description,
  state = 'ready',
  error,
  onRetry,
  empty,
  periods,
  period,
  onPeriodChange,
  legend,
  height = 240,
  children,
  className = '',
}) {
  const theme = useChartTheme();

  return (
    <section className={`rounded-lg border border-border bg-surface shadow-1 p-5 ${className}`}>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          {title && <h3 className="text-card-title text-text">{title}</h3>}
          {description && <p className="mt-1 text-caption text-muted">{description}</p>}
        </div>
        {Array.isArray(periods) && periods.length > 0 && (
          <SegmentedControl
            size="sm"
            options={periods}
            value={period}
            onChange={onPeriodChange}
            ariaLabel="Period"
          />
        )}
      </header>

      {Array.isArray(legend) && legend.length > 0 && state === 'ready' && (
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1">
          {legend.map((l, i) => (
            <span key={l.label} className="inline-flex items-center gap-1.5 text-caption text-muted">
              <span
                className="h-2 w-2 rounded-xs"
                style={{ backgroundColor: theme.series[(l.series ?? i) % theme.series.length] }}
                aria-hidden="true"
              />
              {l.label}
            </span>
          ))}
        </div>
      )}

      <div className="mt-4" style={{ minHeight: height }}>
        {state === 'loading' ? (
          <Skeleton className="h-full w-full min-h-[inherit]" />
        ) : state === 'error' ? (
          <ErrorState
            title={error?.title || 'Could not load this chart'}
            detail={error?.detail}
            requestId={error?.requestId}
            onRetry={onRetry}
          />
        ) : state === 'empty' ? (
          <EmptyState
            icon={empty?.icon}
            title={empty?.title || 'No data for this period'}
            body={empty?.body}
            action={empty?.action}
          />
        ) : typeof children === 'function' ? (
          children(theme)
        ) : (
          children
        )}
      </div>
    </section>
  );
}

export { ChartFrame };
