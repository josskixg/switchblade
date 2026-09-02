/**
 * Label/value pairs for invoice detail, account detail and request detail.
 * `items` is [{ label, value, mono }] — anything machine-generated is mono.
 */
export default function DescriptionList({ items = [], columns = 1, className = '' }) {
  const grid = columns === 2 ? 'sm:grid-cols-2' : 'grid-cols-1';
  return (
    <dl className={`grid gap-x-8 gap-y-3 ${grid} ${className}`}>
      {items.map((item, i) => (
        <div key={`${item.label}-${i}`} className="flex items-baseline justify-between gap-4 min-w-0">
          <dt className="text-label text-muted shrink-0">{item.label}</dt>
          <dd className={`text-body text-text text-right truncate ${item.mono ? 'font-mono text-code tabular-nums lining-nums' : ''}`}>
            {item.value ?? <span className="text-muted">—</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export { DescriptionList };
