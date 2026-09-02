/*
 * Façade over the design system, plus the few helpers that are Track B's own.
 *
 * Every primitive here comes from components/ui — Track A owns them and they
 * are the single source of truth. This module exists so the billing pages have
 * one import line, and so date/number formatting is not re-invented per page.
 */
export { default as Money, formatUsd, nanoToUsd, NANO_PER_USD } from '../../components/ui/Money';
export { default as Meter } from '../../components/ui/Meter';
export { default as StatTile } from '../../components/ui/StatTile';
export { default as PageHeader } from '../../components/ui/PageHeader';
export { default as EmptyState } from '../../components/ui/EmptyState';
export { default as ErrorState } from '../../components/ui/ErrorState';
export { default as Banner } from '../../components/ui/Banner';
export { default as SegmentedControl } from '../../components/ui/SegmentedControl';
export { default as DescriptionList } from '../../components/ui/DescriptionList';
export { default as PlanBadge, PLAN_TOKENS } from '../../components/ui/PlanBadge';
export { default as JackField } from '../../components/ui/JackField';
export { default as CopyField } from '../../components/ui/CopyField';

import { formatUsd, nanoToUsd } from '../../components/ui/Money';

/** Kept for pages that want a bare string rather than the <Money> element. */
export function formatUSD(value, opts) {
  return formatUsd(Number(value) || 0, opts);
}

export function nanoToUSD(nano) {
  return nanoToUsd(nano);
}

/* ─────────────────────────── time ─────────────────────────── */

/** Backend timestamps are unix seconds; a few legacy columns are ISO strings. */
export function toDate(ts) {
  if (ts === null || ts === undefined || ts === '') return null;
  if (ts instanceof Date) return ts;
  if (typeof ts === 'number') return new Date(ts < 1e12 ? ts * 1000 : ts);
  const asNum = Number(ts);
  if (!Number.isNaN(asNum) && String(ts).trim() !== '') {
    return new Date(asNum < 1e12 ? asNum * 1000 : asNum);
  }
  const d = new Date(ts);
  return Number.isNaN(d.getTime()) ? null : d;
}

export function formatDate(ts) {
  const d = toDate(ts);
  if (!d) return '—';
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}

export function formatDateTime(ts) {
  const d = toDate(ts);
  if (!d) return '—';
  return d.toLocaleString(undefined, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function daysUntil(ts) {
  const d = toDate(ts);
  if (!d) return null;
  return Math.max(0, Math.ceil((d.getTime() - Date.now()) / 86_400_000));
}

/* ─────────────────────────── numbers & errors ─────────────────────────── */

export function formatCompact(n) {
  const v = Number(n) || 0;
  if (Math.abs(v) >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (Math.abs(v) >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
  return String(v);
}

export function apiErrorMessage(err, fallback = 'Something went wrong.') {
  return err?.response?.data?.error || err?.message || fallback;
}
