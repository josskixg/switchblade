import { AlertTriangle } from 'lucide-react';
import Button from './Button';

/**
 * The component that ends the practice of `.catch(() => ({ data: {} }))`
 * rendering a confident, false, all-zero dashboard. If the API failed, say so
 * and show the request id support will ask for.
 */
export default function ErrorState({
  icon: Icon = AlertTriangle,
  title = 'Could not load this data',
  detail,
  requestId,
  onRetry,
  className = '',
}) {
  return (
    <div className={`flex flex-col items-center text-center py-12 px-5 ${className}`}>
      <span className="inline-flex h-[72px] w-[72px] items-center justify-center rounded-full bg-surface-sunken mb-4">
        <Icon className="w-10 h-10 text-danger" strokeWidth={1.5} aria-hidden="true" />
      </span>
      <h3 className="text-card-title text-text">{title}</h3>
      {detail && <p className="mt-1.5 text-body text-muted max-w-[44ch]">{detail}</p>}
      {requestId && (
        <p className="mt-3 text-caption text-muted">
          Request ID <code className="font-mono text-code text-text-secondary bg-surface-sunken rounded-xs px-1.5 py-0.5">{requestId}</code>
        </p>
      )}
      {onRetry && (
        <div className="mt-5">
          <Button variant="secondary" size="md" onClick={onRetry}>Try again</Button>
        </div>
      )}
    </div>
  );
}

export { ErrorState };
