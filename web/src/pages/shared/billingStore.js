import { create } from 'zustand';
import { client } from '../../api/client';
import { formatUsd, nanoToUsd } from '../../components/ui/Money';

/**
 * One shared read of GET /api/billing/account. The shell uses it for the
 * sidebar meter and the global banner; the billing pages reuse the same record
 * so a top-up refreshes every surface at once.
 */
export const useBillingStore = create((set, get) => ({
  account: null,
  loading: false,
  loaded: false,
  error: null,
  /** Set by the 402/403 response interceptor: { status, code, message }. */
  block: null,
  dismissed: false,

  load: async ({ force = false } = {}) => {
    if (get().loading) return;
    if (get().loaded && !force) return;
    set({ loading: true, error: null });
    try {
      const res = await client.get('/api/billing/account');
      const next = { account: res.data, loading: false, loaded: true, error: null };
      // A block is a snapshot of a past refusal. If the fresh account state
      // would be admitted again (top-up landed, new period opened), the banner
      // must come down without waiting for another request to fail.
      if (get().block && !stillBlocked(res.data, get().block)) {
        next.block = null;
        next.dismissed = false;
      }
      set(next);
    } catch (error) {
      // A 403 here means the caller has no tenant in context (legacy API key).
      // That is not a billing block — it just means there is nothing to show.
      set({ account: null, loading: false, loaded: true, error });
    }
  },

  setBlock: (block) => set({ block, dismissed: false }),
  dismiss: () => set({ dismissed: true }),
  clearBlock: () => set({ block: null, dismissed: false }),
}));

/*
 * Two producers reach the same handler. The axios interceptor dispatches
 * `sb:billing-block` with { status, code, message }; the gateway contract
 * dispatches `switchblade:payment-required` with the raw 402 body,
 * { error, code }. Both names are literals so this module keeps working
 * whichever constant the client exposes.
 */
function normalizeBlock(detail) {
  const d = detail || {};
  return {
    status: d.status ?? 402,
    code: d.code || '',
    message: d.message || d.error || '',
  };
}

/**
 * Client-side mirror of billing.Meter.Admit, used only to decide when a
 * recorded refusal is stale. tenant_inactive can be lifted only by an
 * operator, so it never clears from account data alone.
 */
function stillBlocked(account, block) {
  if (!block) return false;
  if (!account) return true;
  if (block.code === 'tenant_inactive' || block.status === 403) return true;

  const balance = Number(account.balance_nano) || 0;
  const sub = account.subscription;
  const now = Date.now() / 1000;
  const allowanceOpen =
    sub &&
    sub.status === 'active' &&
    now < Number(sub.period_end) &&
    Number(sub.used_nano) < Number(sub.included_nano);
  if (allowanceOpen) return false;

  const overageAllowed = !sub || sub.overage_enabled !== false;
  return !(overageAllowed && balance > 0);
}

if (typeof window !== 'undefined') {
  const onBlock = (e) => {
    useBillingStore.getState().setBlock(normalizeBlock(e.detail));
    useBillingStore.getState().load({ force: true });
  };
  window.addEventListener('sb:billing-block', onBlock);
  window.addEventListener('switchblade:payment-required', onBlock);
}

/** Derived banner state — low balance, exhausted allowance, suspended tenant. */
export function bannerFor(account, block) {
  const balance = Number(account?.balance_nano) || 0;
  const balanceStr = formatUsd(nanoToUsd(balance));
  const sub = account?.subscription;

  if (block) {
    if (block.status === 403 || block.code === 'tenant_inactive') {
      return {
        tone: 'danger',
        title: 'Your account is not active',
        body: block.message || 'API requests are blocked. Contact support to restore access.',
        to: '/app/billing',
        cta: 'View billing',
        persistent: true,
      };
    }
    if (block.code === 'allowance_exhausted') {
      return {
        tone: 'danger',
        title: 'Plan allowance used up',
        body: `Overage is disabled on this plan, so requests are rejected until a new period starts. Credit balance: ${balanceStr}.`,
        to: '/app/billing',
        cta: 'View plan',
        persistent: false,
      };
    }
    // no_credit — a lapsed period reads differently from a spent allowance,
    // even though Admit returns the same code for both.
    const lapsed = sub && (sub.status !== 'active' || Number(sub.period_end) < Date.now() / 1000);
    if (lapsed) {
      return {
        tone: 'danger',
        title: 'Your plan period has ended',
        body: `Requests now draw on credit, and the balance is ${balanceStr}. Add credit to keep serving requests until a new period starts.`,
        to: '/app/billing/topup',
        cta: 'Add credit',
        persistent: false,
      };
    }
    return {
      tone: 'danger',
      title: sub ? 'Allowance spent and credit is empty' : 'Out of credit',
      body: `API requests are being rejected. Credit balance: ${balanceStr}. Add credit to resume immediately.`,
      to: '/app/billing/topup',
      cta: 'Add credit',
      persistent: false,
    };
  }

  if (!account) return null;

  if (sub && sub.included_nano > 0) {
    const ratio = Number(sub.used_nano) / Number(sub.included_nano);
    if (ratio >= 1 && balance <= 0) {
      return {
        tone: 'danger',
        title: 'Plan allowance used up',
        body: 'Add credit to cover overage, or move to a larger plan.',
        to: '/app/billing/topup',
        cta: 'Add credit',
        persistent: true,
      };
    }
    if (ratio >= 0.8 && ratio < 1) {
      return {
        tone: 'warning',
        title: `You have used ${Math.round(ratio * 100)}% of this period's allowance`,
        body: 'Overage is billed against your credit balance.',
        to: '/app/billing',
        cta: 'View plan',
        persistent: false,
      };
    }
  }

  if (!sub && balance <= 0) {
    return {
      tone: 'danger',
      title: 'No credit on this account',
      body: 'API requests will be rejected until credit is added.',
      to: '/app/billing/topup',
      cta: 'Add credit',
      persistent: true,
    };
  }

  if (balance > 0 && balance < 5 * 1_000_000_000) {
    return {
      tone: 'warning',
      title: 'Credit balance is running low',
      body: 'Top up to avoid interrupted requests.',
      to: '/app/billing/topup',
      cta: 'Add credit',
      persistent: false,
    };
  }

  return null;
}
