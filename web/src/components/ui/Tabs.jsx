import { useSearchParams } from 'react-router-dom';

/**
 * URL-synced tabs. Thirteen sidebar deep-links used to change the URL and do
 * nothing, because no page in the app read a query param — so the tab state
 * lives in the URL here, and deep links, sharing and the back button all work.
 *
 * `tabs` is [{ id, label, count, icon }].
 * Pass `value` + `onChange` to opt out of URL sync (e.g. inside a modal).
 */
export default function Tabs({
  tabs = [],
  param = 'tab',
  value,
  onChange,
  ariaLabel = 'Sections',
  className = '',
}) {
  const [searchParams, setSearchParams] = useSearchParams();
  const controlled = value != null;

  const fallback = tabs[0]?.id;
  const fromUrl = searchParams.get(param);
  const active = controlled
    ? value
    : tabs.some((t) => t.id === fromUrl)
      ? fromUrl
      : fallback;

  function select(id) {
    if (controlled) {
      onChange?.(id);
      return;
    }
    const next = new URLSearchParams(searchParams);
    next.set(param, id);
    setSearchParams(next, { replace: true });
    onChange?.(id);
  }

  return (
    <div role="tablist" aria-label={ariaLabel} className={`flex items-center gap-1 border-b border-border-subtle overflow-x-auto ${className}`}>
      {tabs.map((t) => {
        const selected = t.id === active;
        const Icon = t.icon;
        return (
          <button
            key={t.id}
            type="button"
            role="tab"
            id={`tab-${t.id}`}
            aria-selected={selected}
            aria-controls={`panel-${t.id}`}
            onClick={() => select(t.id)}
            className={`
              relative inline-flex items-center gap-2 h-9 px-3 text-label whitespace-nowrap -mb-px
              border-b-2 transition-[color,border-color] duration-120 ease-swift
              ${selected ? 'border-primary text-primary' : 'border-transparent text-muted hover:text-text'}
            `}
          >
            {Icon && <Icon className="w-4 h-4" aria-hidden="true" />}
            {t.label}
            {t.count != null && (
              <span className="rounded-xs bg-surface-sunken px-1.5 text-caption tabular-nums lining-nums text-muted">
                {t.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}

export function TabPanel({ id, active, children, className = '' }) {
  if (id !== active) return null;
  return (
    <div role="tabpanel" id={`panel-${id}`} aria-labelledby={`tab-${id}`} className={className}>
      {children}
    </div>
  );
}

/** Reads the active tab id from the URL without rendering the tab strip. */
export function useActiveTab(tabs = [], param = 'tab') {
  const [searchParams] = useSearchParams();
  const fromUrl = searchParams.get(param);
  return tabs.some((t) => (t.id ?? t) === fromUrl) ? fromUrl : (tabs[0]?.id ?? tabs[0]);
}

export { Tabs };
