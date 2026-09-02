import PlanBadge from './PlanBadge';

/**
 * Superseded by PlanBadge. Kept as a thin alias so the pages that still import
 * it keep working; new code should import PlanBadge directly.
 */
export default function TierBadge({ tier, className = '' }) {
  return <PlanBadge plan={tier} className={className} />;
}

export { TierBadge };
