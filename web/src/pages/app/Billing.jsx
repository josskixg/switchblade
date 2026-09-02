import { useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowRight, CreditCard, Gauge, ScrollText, Wallet } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import Skeleton from '../../components/ui/Skeleton';
import { useAsync } from '../shared/data';
import { useBillingStore } from '../shared/billingStore';
import { LedgerTable } from '../shared/LedgerTable';
import {
  DescriptionList, EmptyState, ErrorState, Meter, Money, PageHeader, PlanBadge, StatTile,
  apiErrorMessage, daysUntil, formatDate,
} from '../shared/ui';

export default function Billing() {
  const navigate = useNavigate();
  const account = useBillingStore((s) => s.account);
  const loaded = useBillingStore((s) => s.loaded);
  const error = useBillingStore((s) => s.error);
  const load = useBillingStore((s) => s.load);

  useEffect(() => {
    load({ force: true });
  }, [load]);

  const ledger = useAsync(() => client.get('/api/billing/ledger').then((r) => r.data), []);

  if (!loaded) return <BillingSkeleton />;

  if (error) {
    return (
      <div className="space-y-8">
        <PageHeader title="Billing" description="Plan, allowance, and prepaid credit." />
        <Card>
          <ErrorState
            title="Billing is unavailable"
            detail={apiErrorMessage(error, 'The billing service did not respond.')}
            onRetry={() => load({ force: true })}
          />
        </Card>
      </div>
    );
  }

  const sub = account?.subscription;
  const balanceNano = Number(account?.balance_nano) || 0;
  const usedNano = Number(sub?.used_nano) || 0;
  const includedNano = Number(sub?.included_nano) || 0;
  const overageNano = Math.max(0, usedNano - includedNano);
  const remainingNano = Math.max(0, includedNano - usedNano);
  const pct = includedNano > 0 ? Math.round((usedNano / includedNano) * 100) : null;
  const days = daysUntil(sub?.period_end);
  const recent = Array.isArray(ledger.data) ? ledger.data.slice(0, 6) : [];

  return (
    <div className="space-y-8">
      <PageHeader
        title="Billing"
        description="Plan, allowance, and prepaid credit."
        actions={
          <Link to="/app/billing/topup">
            <Button type="button" variant="primary" size="md">
              <CreditCard className="h-4 w-4" aria-hidden="true" />
              Add credit
            </Button>
          </Link>
        }
      />

      {/* ── The meter is the hero ─────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-5">
        <Card className="lg:col-span-2">
          {sub ? (
            <>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <PlanBadge plan={sub.tier_id} />
                  <Badge variant={sub.status === 'active' ? 'success' : 'warning'}>
                    {sub.status}
                  </Badge>
                </div>
                <span className="text-caption text-muted">
                  Period {formatDate(sub.period_start)} – {formatDate(sub.period_end)}
                </span>
              </div>

              <div className="mt-8">
                <Meter
                  size="hero"
                  label="Allowance used"
                  usedNano={usedNano}
                  includedNano={includedNano}
                  periodEnd={sub.period_end}
                />
              </div>

              <p className="mt-4 text-body text-muted">
                {overageNano > 0 ? (
                  <>
                    You are past the included allowance. Overage of{' '}
                    <Money nano={overageNano} className="text-text" /> is billed against your
                    credit balance.
                  </>
                ) : pct !== null ? (
                  <>
                    {pct}% of this period&apos;s allowance is used
                    {days !== null && `, with ${days} ${days === 1 ? 'day' : 'days'} to go`}.
                    Usage past the allowance draws on credit.
                  </>
                ) : (
                  'This plan has no included allowance — every request is billed to credit.'
                )}
              </p>
            </>
          ) : (
            <EmptyState
              icon={Gauge}
              title="No plan on this account"
              body="Requests are billed directly against prepaid credit. Ask your operator to assign a plan if you want a monthly included allowance."
              action={{ label: 'Add credit', onClick: () => navigate('/app/billing/topup') }}
            />
          )}
        </Card>

        {/* Balance — the one place a serif figure is allowed. */}
        <Card className="flex flex-col justify-between">
          <div>
            <div className="flex items-center gap-2">
              <Wallet className="h-4 w-4 text-muted" aria-hidden="true" />
              <p className="text-label text-muted">Credit balance</p>
            </div>
            <p className="mt-6">
              <Money
                nano={balanceNano}
                size="hero"
                sign="none"
                tone={balanceNano <= 0 ? 'danger' : undefined}
              />
            </p>
            <p className="mt-2 text-caption text-muted">
              {balanceNano <= 0
                ? 'Requests are rejected once the allowance is spent.'
                : 'Covers overage and any usage outside the plan.'}
            </p>
          </div>
          <div className="mt-6">
            <Link to="/app/billing/topup" className="block">
              <Button type="button" variant="secondary" size="md" className="w-full">
                Add credit
              </Button>
            </Link>
          </div>
        </Card>
      </div>

      {/* ── Period detail ─────────────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
        <StatTile label="Used this period" nano={usedNano} />
        <StatTile
          label="Allowance left"
          nano={remainingNano}
          empty={includedNano === 0}
          emptyLabel="No plan allowance"
        />
        <StatTile
          label="Overage"
          nano={overageNano}
          empty={sub?.overage_enabled === false}
          emptyLabel="Overage disabled"
        />
        <StatTile
          label="Renews in"
          value={days}
          unit={days === 1 ? 'day' : 'days'}
          empty={days === null}
          emptyLabel="No active period"
        />
      </div>

      {/* ── Plan facts ────────────────────────────────────────────── */}
      {sub && (
        <Card title="Plan" description="What this account is entitled to right now.">
          <DescriptionList
            columns={2}
            items={[
              { label: 'Plan', value: <PlanBadge plan={sub.tier_id} /> },
              { label: 'Status', value: sub.status },
              { label: 'Included allowance', value: <Money nano={includedNano} sign="none" /> },
              {
                label: 'Overage billing',
                value: sub.overage_enabled ? 'Enabled — draws on credit' : 'Disabled',
              },
              { label: 'Period ends', value: formatDate(sub.period_end) },
              { label: 'Account', value: account?.tenant_id || '—', mono: true },
            ]}
          />
        </Card>
      )}

      {/* ── Ledger preview ────────────────────────────────────────── */}
      <Card
        title="Recent activity"
        description="Every charge and credit, newest first."
        headerAction={
          <Link to="/app/billing/ledger">
            <Button type="button" variant="ghost" size="sm">
              Full ledger
              <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
            </Button>
          </Link>
        }
      >
        {ledger.loading ? (
          <div className="space-y-2">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-11 w-full" />
            ))}
          </div>
        ) : ledger.error ? (
          <ErrorState
            title="Could not load the ledger"
            detail={apiErrorMessage(ledger.error)}
            onRetry={ledger.reload}
          />
        ) : recent.length === 0 ? (
          <EmptyState
            icon={ScrollText}
            title="Nothing billed yet"
            body="Charges appear here the moment your first request is metered."
            action={{ label: 'Create an API key', onClick: () => navigate('/app/keys') }}
          />
        ) : (
          <LedgerTable entries={recent} showRequest={false} />
        )}
      </Card>
    </div>
  );
}

function BillingSkeleton() {
  return (
    <div className="space-y-8">
      <Skeleton className="h-9 w-40" />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Skeleton className="h-56 lg:col-span-2" />
        <Skeleton className="h-56" />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
    </div>
  );
}
