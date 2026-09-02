import { useState } from 'react';
import { AlertTriangle, Info, X, XCircle } from 'lucide-react';

/**
 * Global billing state, between the header and the content. Driven by
 * GET /api/billing/account plus the client-side 402 interceptor.
 *
 * `danger` is not dismissible — credit is exhausted until it is resolved, and
 * hiding that helps nobody.
 */
const tones = {
  info: { wrap: 'bg-primary/8 border-primary/30 text-text', icon: 'text-primary', Icon: Info },
  warning: { wrap: 'bg-warning/10 border-warning/30 text-text', icon: 'text-warning', Icon: AlertTriangle },
  danger: { wrap: 'bg-danger/10 border-danger/30 text-text', icon: 'text-danger', Icon: XCircle },
};

export default function Banner({
  tone = 'info',
  title,
  children,
  action,
  dismissible,
  onDismiss,
  className = '',
}) {
  const [hidden, setHidden] = useState(false);
  const t = tones[tone] || tones.info;
  const canDismiss = dismissible ?? tone !== 'danger';

  if (hidden) return null;

  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      className={`flex items-start gap-3 border-b px-4 py-3 lg:px-6 ${t.wrap} ${className}`}
    >
      <t.Icon className={`w-4 h-4 mt-0.5 shrink-0 ${t.icon}`} aria-hidden="true" />
      <div className="flex-1 min-w-0 text-body">
        {title && <span className="font-medium">{title} </span>}
        {children && <span className="text-text-secondary">{children}</span>}
      </div>
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="shrink-0 text-label text-primary hover:underline"
        >
          {action.label}
        </button>
      )}
      {canDismiss && (
        <button
          type="button"
          onClick={() => {
            setHidden(true);
            onDismiss?.();
          }}
          aria-label="Dismiss"
          className="shrink-0 inline-flex h-6 w-6 items-center justify-center rounded-xs text-muted hover:bg-surface-hover hover:text-text transition-[background-color,color] duration-120 ease-swift"
        >
          <X className="w-3.5 h-3.5" aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

export { Banner };
