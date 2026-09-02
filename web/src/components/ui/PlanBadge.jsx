import Badge from './Badge';

/**
 * One map, semantic tokens only. Replaces the two competing tier→color maps
 * in TierBadge and QuotaBar, which injected raw Tailwind 400-shades measuring
 * 1.67:1, 2.54:1 and ~2.0:1 on white.
 */
export const PLAN_TOKENS = {
  free: { label: 'Free', variant: 'neutral' },
  trial: { label: 'Trial', variant: 'info' },
  cheap: { label: 'Metered', variant: 'default' },
  starter: { label: 'Starter', variant: 'info' },
  pro: { label: 'Pro', variant: 'primary' },
  subscription: { label: 'Subscription', variant: 'primary' },
  business: { label: 'Business', variant: 'primary' },
  enterprise: { label: 'Enterprise', variant: 'primary' },
  suspended: { label: 'Suspended', variant: 'danger' },
  past_due: { label: 'Past due', variant: 'warning' },
};

function titleCase(s) {
  return String(s || '')
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export default function PlanBadge({ plan, tier, label, size = 'md', className = '' }) {
  const key = String(plan ?? tier ?? '').toLowerCase();
  const cfg = PLAN_TOKENS[key] || { label: label || titleCase(key) || 'Unknown', variant: 'default' };
  return (
    <Badge variant={cfg.variant} size={size} className={className}>
      {label || cfg.label}
    </Badge>
  );
}

export { PlanBadge };
