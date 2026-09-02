import { useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowRight, BookOpen, KeyRound, Layers, Wallet } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import Skeleton from '../../components/ui/Skeleton';
import { useAuthStore } from '../../store/auth';
import { useAsync } from '../shared/data';
import { useBillingStore } from '../shared/billingStore';
import { LedgerTable } from '../shared/LedgerTable';
import {
  EmptyState, ErrorState, Meter, Money, PageHeader, PlanBadge, StatTile,
  apiErrorMessage, daysUntil, formatDate,
} from '../shared/ui';

export default function Home() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const account = useBillingStore((s) => s.account);
  const billingLoaded = useBillingStore((s) => s.loaded);
  const billingError = useBillingStore((s) => s.error);
  const loadBilling = useBillingStore((s) => s.load);
  const billingLoading = !billingLoaded;

  useEffect(() => {
    loadBilling({ force: true });
  }, [loadBilling]);

  const ledger = useAsync(
    () => client.get('/api/billing/ledger').then((r) => r.data),
    []
  );

  const entries = Array.isArray(ledger.data) ? ledger.data : [];
  const usageEntries = entries.filter((e) => e.kind === 'usage' || e.kind === 'subscription');
  const hasActivity = usageEntries.length > 0;

  const sub = account?.subscription;
  const usedNano = Number(sub?.used_nano) || 0;
  const includedNano = Number(sub?.included_nano) || 0;
  const overageNano = Math.max(0, usedNano - includedNano);
  const days = daysUntil(sub?.period_end);

  return (
    <div className="space-y-8">
      <PageHeader
        title={`Welcome back, ${user?.username || 'there'}`}
        description="Your gateway at a glance."
        actions={
          <Link to="/app/keys">
            <Button type="button" variant="primary" size="md">
              <KeyRound className="h-4 w-4" aria-hidden="true" />
              API keys
            </Button>
          </Link>
        }
      />

      {/* ── Allowance + balance ───────────────────────────────────── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-5">
        <Card className="lg:col-span-2">
          {billingLoading && !account ? (
            <div className="space-y-3">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-3 w-full" />
              <Skeleton className="h-4 w-48" />
            </div>
          ) : billingError ? (
            <ErrorState
              title="Billing is unavailable"
              detail={apiErrorMessage(billingError)}
              onRetry={() => loadBilling({ force: true })}
            />
          ) : sub ? (
            <>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <p className="text-card-title text-text">This billing period</p>
                  <PlanBadge plan={sub.tier_id} />
                </div>
                <Link
                  to="/app/billing"
                  className="inline-flex items-center gap-1 text-label text-primary"
                >
                  Billing
                  <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
                </Link>
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
            </>
          ) : (
            <EmptyState
              icon={Wallet}
              title="Billed from credit"
              body="There is no plan on this account, so every request draws directly on your prepaid balance."
              action={{ label: 'View billing', onClick: () => navigate('/app/billing') }}
            />
          )}
        </Card>

        <Card className="flex flex-col justify-between">
          <div>
            <p className="text-label text-muted">Credit balance</p>
            <p className="mt-6">
              <Money
                nano={account?.balance_nano || 0}
                size="hero"
                sign="none"
                tone={(Number(account?.balance_nano) || 0) <= 0 ? 'danger' : undefined}
              />
            </p>
          </div>
          <Link to="/app/billing/topup" className="mt-6 block">
            <Button type="button" variant="secondary" size="md" className="w-full">
              Add credit
            </Button>
          </Link>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
        <StatTile
          label="Spend this period"
          nano={usedNano}
          loading={billingLoading}
          empty={Boolean(billingError)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Overage"
          nano={overageNano}
          loading={billingLoading}
          empty={Boolean(billingError)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Period ends"
          value={sub?.period_end ? formatDate(sub.period_end) : null}
          unit={days !== null ? `${days} ${days === 1 ? 'day' : 'days'} left` : undefined}
          loading={billingLoading}
          empty={Boolean(billingError) || !sub?.period_end}
          emptyLabel={billingError ? 'Unavailable' : 'No active period'}
        />
        <StatTile
          label="Ledger entries"
          value={entries.length}
          unit="of last 200"
          loading={ledger.loading}
          empty={Boolean(ledger.error)}
          emptyLabel="Unavailable"
        />
      </div>

      {/* ── First-run path, or recent charges ─────────────────────── */}
      {!ledger.loading && !ledger.error && !hasActivity ? (
        <Card title="Send your first request" description="Three steps to a live integration.">
          <ol className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Step
              n="1"
              icon={KeyRound}
              title="Create an API key"
              body="Keys are shown once at creation and authenticate every call."
              to="/app/keys"
              cta="Create a key"
            />
            <Step
              n="2"
              icon={Layers}
              title="Pick a model"
              body="Browse the catalog and see exactly what each model costs."
              to="/app/models"
              cta="Browse models"
            />
            <Step
              n="3"
              icon={BookOpen}
              title="Point your client at us"
              body="Drop-in for the OpenAI SDK, cURL, Cursor, and Cline."
              to="/app/docs"
              cta="Read the docs"
            />
          </ol>
        </Card>
      ) : (
        <Card
          title="Recent charges"
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
              title="Could not load recent charges"
              detail={apiErrorMessage(ledger.error)}
              onRetry={ledger.reload}
            />
          ) : (
            <LedgerTable entries={entries.slice(0, 6)} showBalance={false} />
          )}
        </Card>
      )}
    </div>
  );
}

function Step({ n, icon: Icon, title, body, to, cta }) {
  return (
    <li className="rounded-[var(--r-md,8px)] border border-border-subtle bg-surface-sunken p-4">
      <div className="flex items-center gap-2">
        <span className="flex h-6 w-6 items-center justify-center rounded-[var(--r-xs,4px)] bg-surface text-micro text-muted">
          {n}
        </span>
        <Icon className="h-4 w-4 text-muted" aria-hidden="true" />
      </div>
      <p className="mt-3 text-card-title text-text">{title}</p>
      <p className="mt-1 text-body text-muted">{body}</p>
      <Link to={to} className="mt-3 inline-flex items-center gap-1 text-label text-primary">
        {cta}
        <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />
      </Link>
    </li>
  );
}
