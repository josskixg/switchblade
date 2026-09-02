import { useRef, useState } from 'react';

/**
 * The operator signature: a patch bay. One port per pooled account, lit by
 * health state. It answers the operator's only real question — how much
 * capacity is actually live right now.
 */
const portStates = {
  active: { cls: 'bg-success', label: 'Active' },
  'in-flight': { cls: 'bg-primary ring-1 ring-primary ring-offset-1 ring-offset-surface', label: 'In flight' },
  cooldown: { cls: 'bg-warning', label: 'Cooldown' },
  error: { cls: 'bg-danger', label: 'Error' },
  disabled: { cls: 'bg-border-strong', label: 'Disabled' },
};

const ORDER = ['active', 'in-flight', 'cooldown', 'error', 'disabled'];

export default function JackField({ ports = [], showCounts = true, className = '' }) {
  const gridRef = useRef(null);
  const [focusIndex, setFocusIndex] = useState(0);
  const [openIndex, setOpenIndex] = useState(null);

  const counts = ports.reduce((acc, p) => {
    const s = portStates[p.state] ? p.state : 'disabled';
    acc[s] = (acc[s] || 0) + 1;
    return acc;
  }, {});

  function columnCount() {
    const grid = gridRef.current;
    if (!grid || grid.children.length === 0) return 1;
    const firstTop = grid.children[0].offsetTop;
    let n = 0;
    for (const child of grid.children) {
      if (child.offsetTop !== firstTop) break;
      n += 1;
    }
    return Math.max(1, n);
  }

  function onKeyDown(e, i) {
    const cols = columnCount();
    let next = null;
    if (e.key === 'ArrowRight') next = Math.min(ports.length - 1, i + 1);
    else if (e.key === 'ArrowLeft') next = Math.max(0, i - 1);
    else if (e.key === 'ArrowDown') next = Math.min(ports.length - 1, i + cols);
    else if (e.key === 'ArrowUp') next = Math.max(0, i - cols);
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = ports.length - 1;
    else if (e.key === 'Escape') {
      setOpenIndex(null);
      return;
    }
    if (next == null) return;
    e.preventDefault();
    setFocusIndex(next);
    gridRef.current?.children[next]?.querySelector('button')?.focus();
  }

  if (ports.length === 0) {
    return (
      <p className={`text-body text-muted ${className}`}>
        No accounts in the pool yet.
      </p>
    );
  }

  return (
    <div className={className}>
      {showCounts && (
        <div className="mb-3 flex flex-wrap items-center gap-x-4 gap-y-1">
          {ORDER.filter((s) => counts[s]).map((s) => (
            <span key={s} className="inline-flex items-center gap-1.5 text-micro uppercase text-muted">
              <span className={`h-2 w-2 rounded-xs ${portStates[s].cls}`} aria-hidden="true" />
              {counts[s]} {portStates[s].label}
            </span>
          ))}
        </div>
      )}

      <div
        ref={gridRef}
        role="group"
        aria-label="Account pool"
        className="flex flex-wrap gap-1"
      >
        {ports.map((p, i) => {
          const state = portStates[p.state] ? p.state : 'disabled';
          const cfg = portStates[state];
          const open = openIndex === i;
          return (
            <span key={p.id ?? i} className="relative">
              <button
                type="button"
                tabIndex={i === focusIndex ? 0 : -1}
                onKeyDown={(e) => onKeyDown(e, i)}
                onFocus={() => {
                  setFocusIndex(i);
                  setOpenIndex(i);
                }}
                onBlur={() => setOpenIndex((cur) => (cur === i ? null : cur))}
                onMouseEnter={() => setOpenIndex(i)}
                onMouseLeave={() => setOpenIndex((cur) => (cur === i ? null : cur))}
                aria-label={`${p.provider || 'Account'} — ${cfg.label}${p.label ? ` — ${p.label}` : ''}`}
                className={`block h-2.5 w-2.5 rounded-xs ${cfg.cls}`}
              />
              {open && (
                <span
                  role="tooltip"
                  className="absolute bottom-full left-0 z-20 mb-2 w-56 rounded-lg border border-border bg-surface-elevated shadow-2 p-3 text-left"
                >
                  <span className="block text-label text-text">{p.provider || 'Account'}</span>
                  {p.label && (
                    <span className="mt-0.5 block font-mono text-code text-muted truncate">{p.label}</span>
                  )}
                  <span className="mt-2 block text-caption text-muted">
                    State <span className="text-text-secondary">{cfg.label}</span>
                  </span>
                  {p.lastUsed && (
                    <span className="block text-caption text-muted">
                      Last used <span className="text-text-secondary">{p.lastUsed}</span>
                    </span>
                  )}
                  {p.quotaRemaining != null && (
                    <span className="block text-caption text-muted">
                      Quota left <span className="text-text-secondary tabular-nums lining-nums">{Number(p.quotaRemaining).toLocaleString('en-US')}</span>
                    </span>
                  )}
                </span>
              )}
            </span>
          );
        })}
      </div>
    </div>
  );
}

export { JackField };
