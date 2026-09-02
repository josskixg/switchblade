import { useMemo } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Activity, Receipt, Wallet } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import ChartFrame from '../../components/ui/ChartFrame';
import DataTable from '../../components/ui/DataTable';
import Skeleton from '../../components/ui/Skeleton';
import { tooltipProps } from '../../theme/chartTheme';
import { useAsync } from '../shared/data';
import { LedgerTable } from '../shared/LedgerTable';
import {
  EmptyState, ErrorState, Meter, PageHeader, StatTile,
  apiErrorMessage, formatCompact, formatUsd, nanoToUSD,
} from '../shared/ui';

/*
 * Money on this page is nanodollars end to end — the amounts come off
 * credit_ledger and tenant_subscriptions, and <Money> is the only thing that
 * turns them into currency. usage_records.cost_cents is not read: the meter
 * writes 0 into it and puts the real figure in cost_nano.
 *
 * The tenant-scoped endpoints are /api/billing/*, which resolve the caller's
 * tenant server-side. /api/usage and /api/usage/records take a tenant_id query
 * parameter that no login response exposes, so they are unusable here.
 */

const CHART_DAYS = 14;

function dayKey(ts) {
  const d = new Date(Number(ts) * 1000);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function dayLabel(key) {
  const [y, m, d] = key.split('-').map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
}

/** Debits only — a top-up is not usage. Ledger amounts are negative for spend. */
function isSpend(entry) {
  return entry.kind === 'usage' || entry.kind === 'subscription';
}

export default function Usage() {
  const isFleet = useLocation().pathname.startsWith('/admin');
  return isFleet ? <FleetUsage /> : <TenantUsage />;
}

/* ── Customer view: what this tenant spent ─────────────────────────── */

function TenantUsage() {
  const navigate = useNavigate();
  const account = useAsync(() => client.get('/api/billing/account').then((r) => r.data), []);
  const ledger = useAsync(() => client.get('/api/billing/ledger').then((r) => r.data), []);

  const entries = useMemo(() => (Array.isArray(ledger.data) ? ledger.data : []), [ledger.data]);
  const spend = useMemo(() => entries.filter(isSpend), [entries]);

  const sub = account.data?.subscription;
  const includedNano = Number(sub?.included_nano) || 0;
  const usedNano = Number(sub?.used_nano) || 0;
  const remainingNano = Math.max(0, includedNano - usedNano);
  const balanceNano = Number(account.data?.balance_nano) || 0;

  const series = useMemo(() => {
    const buckets = new Map();
    const cutoff = Date.now() / 1000 - CHART_DAYS * 86400;
    for (const e of spend) {
      if (Number(e.created_at) < cutoff) continue;
      const key = dayKey(e.created_at);
      buckets.set(key, (buckets.get(key) || 0) + Math.abs(Number(e.amount_nano) || 0));
    }
    return Array.from(buckets, ([key, nano]) => ({ key, day: dayLabel(key), usd: nanoToUSD(nano) }))
      .sort((a, b) => a.key.localeCompare(b.key));
  }, [spend]);

  const chartState = ledger.loading
    ? 'loading'
    : ledger.error
      ? 'error'
      : series.length === 0
        ? 'empty'
        : 'ready';

  return (
    <div className="space-y-8">
      <PageHeader title="Usage" description="What your requests have cost, drawn from the billing ledger." />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
        <StatTile
          label="Allowance used"
          nano={usedNano}
          loading={account.loading}
          empty={Boolean(account.error) || !sub}
          emptyLabel={account.error ? 'Unavailable' : 'No plan'}
        />
        <StatTile
          label="Allowance remaining"
          nano={remainingNano}
          loading={account.loading}
          empty={Boolean(account.error) || !sub}
          emptyLabel={account.error ? 'Unavailable' : 'No plan'}
        />
        <StatTile
          label="Credit balance"
          nano={balanceNano}
          icon={Wallet}
          loading={account.loading}
          empty={Boolean(account.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Charged requests"
          value={spend.length}
          unit={entries.length ? `of last ${entries.length} entries` : undefined}
          icon={Receipt}
          loading={ledger.loading}
          empty={Boolean(ledger.error)}
          emptyLabel="Unavailable"
        />
      </div>

      <Card title="This billing period">
        {account.loading ? (
          <Skeleton className="h-16 w-full" />
        ) : account.error ? (
          <ErrorState
            title="Billing is unavailable"
            detail={apiErrorMessage(account.error)}
            onRetry={account.reload}
          />
        ) : sub ? (
          <Meter size="hero" usedNano={usedNano} includedNano={includedNano} periodEnd={sub.period_end} />
        ) : (
          <EmptyState
            icon={Wallet}
            title="Billed from credit"
            body="There is no plan on this account, so every request draws directly on your prepaid balance."
            action={{ label: 'Add credit', onClick: () => navigate('/app/billing/topup') }}
          />
        )}
      </Card>

      <ChartFrame
        title={`Daily spend (${CHART_DAYS} days)`}
        description="Allowance and credit charges, by the day they were billed."
        state={chartState}
        error={{ title: 'Could not load the ledger', detail: apiErrorMessage(ledger.error) }}
        onRetry={ledger.reload}
        empty={{
          icon: Activity,
          title: 'No charges in this window',
          body: 'Spend appears here once metered requests are billed.',
        }}
      >
        {(theme) => (
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={series}>
              <CartesianGrid vertical={false} stroke={theme.grid} />
              <XAxis
                dataKey="day"
                tick={{ fill: theme.axis, fontSize: 11 }}
                axisLine={{ stroke: theme.axisLine }}
                tickLine={false}
              />
              <YAxis
                tick={{ fill: theme.axis, fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                tickFormatter={(v) => formatUsd(v, { maxFractionDigits: 2 })}
              />
              <Tooltip {...tooltipProps(theme)} formatter={(v) => formatUsd(v)} />
              <Bar dataKey="usd" name="Spend" fill={theme.series[0]} radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </ChartFrame>

      <Card title="Recent charges" padded={false}>
        {ledger.loading ? (
          <div className="space-y-2 p-5">
            {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-11 w-full" />)}
          </div>
        ) : ledger.error ? (
          <ErrorState
            title="Could not load recent charges"
            detail={apiErrorMessage(ledger.error)}
            onRetry={ledger.reload}
          />
        ) : spend.length === 0 ? (
          <EmptyState
            icon={Receipt}
            title="No charges yet"
            body="Every billed request lands here with the model and request id it belongs to."
          />
        ) : (
          <LedgerTable entries={spend.slice(0, 20)} showBalance={false} />
        )}
      </Card>
    </div>
  );
}

/* ── Operator view: fleet-wide rollup ──────────────────────────────── */

function FleetUsage() {
  const stats = useAsync(() => client.get('/api/stats').then((r) => r.data), []);
  const rows = useMemo(() => (Array.isArray(stats.data) ? stats.data : []), [stats.data]);

  const totalRequests = rows.reduce((s, r) => s + (Number(r.request_count) || 0), 0);
  const totalTokens = rows.reduce((s, r) => s + (Number(r.total_tokens) || 0), 0);
  const providers = new Set(rows.map((r) => r.provider).filter(Boolean)).size;

  const byModel = useMemo(
    () =>
      rows
        .map((r) => ({ model: r.model || 'unknown', tokens: Number(r.total_tokens) || 0 }))
        .sort((a, b) => b.tokens - a.tokens)
        .slice(0, 10),
    [rows]
  );

  const chartState = stats.loading
    ? 'loading'
    : stats.error
      ? 'error'
      : byModel.length === 0
        ? 'empty'
        : 'ready';

  return (
    <div className="space-y-8">
      <PageHeader title="Usage" description="Recorded throughput across every tenant." />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
        <StatTile
          label="Requests recorded"
          value={formatCompact(totalRequests)}
          icon={Activity}
          loading={stats.loading}
          empty={Boolean(stats.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Tokens recorded"
          value={formatCompact(totalTokens)}
          loading={stats.loading}
          empty={Boolean(stats.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Models seen"
          value={rows.length}
          loading={stats.loading}
          empty={Boolean(stats.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Providers"
          value={providers}
          loading={stats.loading}
          empty={Boolean(stats.error)}
          emptyLabel="Unavailable"
        />
      </div>

      <ChartFrame
        title="Tokens by model"
        description="From the usage_summary rollup."
        state={chartState}
        error={{ title: 'Could not load the rollup', detail: apiErrorMessage(stats.error) }}
        onRetry={stats.reload}
        empty={{
          icon: Activity,
          title: 'No traffic recorded yet',
          body: 'Rollups appear as soon as requests are metered through the gateway.',
        }}
      >
        {(theme) => (
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={byModel} layout="vertical" margin={{ left: 8, right: 8 }}>
              <CartesianGrid horizontal={false} stroke={theme.grid} />
              <XAxis
                type="number"
                tick={{ fill: theme.axis, fontSize: 11 }}
                axisLine={{ stroke: theme.axisLine }}
                tickLine={false}
              />
              <YAxis
                type="category"
                dataKey="model"
                width={140}
                tick={{ fill: theme.axis, fontSize: 11 }}
                axisLine={false}
                tickLine={false}
              />
              <Tooltip {...tooltipProps(theme)} />
              <Bar dataKey="tokens" name="Tokens" fill={theme.series[0]} radius={[0, 4, 4, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </ChartFrame>

      <DataTable
        state={stats.loading ? 'loading' : stats.error ? 'error' : 'ready'}
        error={{ title: 'Could not load usage', detail: apiErrorMessage(stats.error) }}
        onRetry={stats.reload}
        empty={{ icon: Activity, title: 'No usage recorded yet' }}
        rows={rows}
        rowKey={(r, i) => `${r.provider}:${r.model}:${i}`}
        columns={[
          { key: 'provider', header: 'Provider' },
          {
            key: 'model',
            header: 'Model',
            render: (r) => <span className="font-mono text-code text-text">{r.model || '—'}</span>,
          },
          { key: 'request_count', header: 'Requests', align: 'numeric' },
          {
            key: 'prompt_tokens',
            header: 'Prompt',
            align: 'numeric',
            render: (r) => formatCompact(r.prompt_tokens),
          },
          {
            key: 'completion_tokens',
            header: 'Completion',
            align: 'numeric',
            render: (r) => formatCompact(r.completion_tokens),
          },
          {
            key: 'total_tokens',
            header: 'Tokens',
            align: 'numeric',
            render: (r) => formatCompact(r.total_tokens),
          },
        ]}
      />
    </div>
  );
}
