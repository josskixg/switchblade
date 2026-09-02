/**
 * Theme switch (System / Light / Dark), period pickers (24h / 7d / 30d), and
 * compact tab bars.
 *
 * `options` is [{ value, label, icon }].
 */
export default function SegmentedControl({
  options = [],
  value,
  onChange,
  size = 'md',
  ariaLabel,
  className = '',
}) {
  const h = size === 'sm' ? 'h-7' : 'h-8';
  const pad = size === 'sm' ? 'px-2' : 'px-3';
  const text = size === 'sm' ? 'text-caption' : 'text-label';

  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className={`inline-flex items-center gap-0.5 rounded-md bg-surface-sunken border border-border-subtle p-0.5 ${className}`}
    >
      {options.map((opt) => {
        const selected = opt.value === value;
        const Icon = opt.icon;
        return (
          <button
            key={opt.value}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => onChange?.(opt.value)}
            className={`
              inline-flex items-center justify-center gap-1.5 rounded-sm whitespace-nowrap
              transition-[background-color,color,box-shadow] duration-120 ease-swift
              ${h} ${pad} ${text}
              ${selected
                ? 'bg-surface text-text shadow-1 dark:bg-surface-hover dark:shadow-none'
                : 'text-muted hover:text-text'}
            `}
          >
            {Icon && <Icon className="w-3.5 h-3.5" aria-hidden="true" />}
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

export { SegmentedControl };
