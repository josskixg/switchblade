import { Inbox } from 'lucide-react';
import Button from './Button';

/**
 * An empty screen is an invitation to act, so `action` is required in spirit:
 * every empty state offers the next step. "Create your first API key",
 * "Send your first request", "Add credit".
 */
export default function EmptyState({
  icon: Icon = Inbox,
  title,
  body,
  action,
  secondaryAction,
  className = '',
}) {
  return (
    <div className={`flex flex-col items-center text-center py-12 px-5 ${className}`}>
      <span className="inline-flex h-[72px] w-[72px] items-center justify-center rounded-full bg-surface-sunken mb-4">
        <Icon className="w-10 h-10 text-muted" strokeWidth={1.5} aria-hidden="true" />
      </span>
      {title && <h3 className="text-card-title text-text">{title}</h3>}
      {body && <p className="mt-1.5 text-body text-muted max-w-[44ch]">{body}</p>}
      {(action || secondaryAction) && (
        <div className="mt-5 flex flex-wrap items-center justify-center gap-2">
          {action && (
            <Button variant="primary" size="md" onClick={action.onClick} {...(action.props || {})}>
              {action.label}
            </Button>
          )}
          {secondaryAction && (
            <Button variant="secondary" size="md" onClick={secondaryAction.onClick} {...(secondaryAction.props || {})}>
              {secondaryAction.label}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

export { EmptyState };
