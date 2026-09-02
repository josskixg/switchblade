// The canonical money renderer. Nothing else in the product formats currency,
// and `cost_cents` is never read — it is hardcoded 0 upstream.
//
// 1 USD = 1_000_000_000 Nano.
export const NANO_PER_USD = 1e9;

export function nanoToUsd(nano) {
  return Number(nano || 0) / NANO_PER_USD;
}

/**
 * Token-priced work produces amounts far below a cent, so precision scales
 * with magnitude rather than being fixed at 2.
 */
export function formatUsd(usd, { currency = 'USD', maxFractionDigits } = {}) {
  const abs = Math.abs(usd);
  let digits = maxFractionDigits;
  if (digits == null) {
    if (abs === 0 || abs >= 1) digits = 2;
    else if (abs >= 0.01) digits = 4;
    else digits = 6;
  }
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits: Math.min(2, digits),
    maximumFractionDigits: digits,
  }).format(usd);
}

const sizeStyles = {
  inline: 'text-body',
  label: 'text-label',
  metric: 'text-metric-md',
  // Statements have been set in serif for two hundred years. Single large
  // figures only — never a table, never a column, never below 24px.
  hero: 'text-display-1 font-display',
};

export default function Money({
  nano,
  usd,
  size = 'inline',
  sign = 'auto',
  currency = 'USD',
  maxFractionDigits,
  tone,
  className = '',
}) {
  const value = usd != null ? Number(usd) : nanoToUsd(nano);
  const formatted = formatUsd(Math.abs(value), { currency, maxFractionDigits });

  let prefix = '';
  if (sign === 'always') prefix = value < 0 ? '−' : '+';
  else if (sign === 'auto' && value < 0) prefix = '−';

  // A debit against credit is the only negative worth coloring.
  const autoTone = sign === 'auto' && value < 0 ? 'text-danger' : '';
  const toneClass = tone === 'danger'
    ? 'text-danger'
    : tone === 'success'
      ? 'text-success'
      : tone === 'warning'
        ? 'text-warning'
        : tone === 'muted'
          ? 'text-muted'
          : autoTone;

  return (
    <span
      className={`tabular-nums lining-nums ${sizeStyles[size] || sizeStyles.inline} ${toneClass} ${className}`}
      title={`${prefix}${formatUsd(Math.abs(value), { currency, maxFractionDigits: 9 })}`}
    >
      {prefix}
      {formatted}
    </span>
  );
}

export { Money };
