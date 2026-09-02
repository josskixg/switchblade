import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Building2, CreditCard, Receipt } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { Badge } from '../../components/ui/Badge';
import Input from '../../components/ui/Input';
import Select from '../../components/ui/Select';
import Skeleton from '../../components/ui/Skeleton';
import { useToast } from '../../components/ui/Toast';
import { useAsync } from '../shared/data';
import { LedgerTable } from '../shared/LedgerTable';
import {
  DescriptionList, EmptyState, ErrorState, Meter, Money, PageHeader, PlanBadge,
  apiErrorMessage, formatDate, formatUSD,
} from '../shared/ui';

export default function TenantBilling() {
  const toast = useToast();
  // URL-synced so an operator can link a colleague straight at a tenant.
  const [params, setParams] = useSearchParams();
  const tenantId = params.get('tenant') || '';

  const tenants = useAsync(() => client.get('/api/tenants').then((r) => r.data), []);
  const tiers = useAsync(() => client.get('/api/tiers').then((r) => r.data), []);

  const account = useAsync(
    () =>
      client
        .get('/api/billing/account', { params: { tenant_id: tenantId } })
        .then((r) => r.data),
    [tenantId],
    { skip: !tenantId }
  );

  const ledger = useAsync(
    () =>
      client
        .get('/api/billing/ledger', { params: { tenant_id: tenantId } })
        .then((r) => r.data),
    [tenantId],
    { skip: !tenantId }
  );

  const tenantList = useMemo(
    () => (Array.isArray(tenants.data) ? tenants.data : []),
    [tenants.data]
  );
  const tierList = useMemo(() => (Array.isArray(tiers.data) ? tiers.data : []), [tiers.data]);
  const selected = tenantList.find((t) => t.id === tenantId);

  function pick(id) {
    if (id) setParams({ tenant: id });
    else setParams({});
  }

  function refreshAll() {
    account.reload();
    ledger.reload();
  }

  const sub = account.data?.subscription;

  return (
    <div className="space-y-8">
      <PageHeader
        title="Tenant billing"
        description="Balance, plan, and credit movements for a single customer."
        actions={
          <div className="w-72">
            <Select
              aria-label="Tenant"
              value={tenantId}
              onChange={(e) => pick(e.target.value)}
              disabled={tenants.loading || Boolean(tenants.error)}
            >
              <option value="">Select a tenant…</option>
              {tenantList.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name || t.id}
                </option>
              ))}
            </Select>
          </div>
        }
      />

      {tenants.error && (
        <Card>
          <ErrorState
            title="Could not list tenants"
            detail={`${apiErrorMessage(tenants.error)} — listing tenants requires the owner role.`}
            onRetry={tenants.reload}
          />
        </Card>
      )}

      {!tenantId ? (
        <Card>
          <EmptyState
            icon={Building2}
            title="Pick a tenant"
            body="Choose a customer to see their balance, plan period, and credit ledger, and to move money."
          />
        </Card>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-5">
            <Card className="lg:col-span-2">
              {account.loading ? (
                <div className="space-y-3">
                  <Skeleton className="h-4 w-40" />
                  <Skeleton className="h-3 w-full" />
                </div>
              ) : account.error ? (
                <ErrorState
                  title="Could not read this account"
                  detail={apiErrorMessage(account.error)}
                  onRetry={account.reload}
                />
              ) : sub ? (
                <>
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div className="flex items-center gap-2">
                      <PlanBadge plan={sub.tier_id} />
                      <Badge variant={sub.status === 'active' ? 'success' : 'warning'}>
                        {sub.status}
                      </Badge>
                      {selected?.status && selected.status !== 'active' && (
                        <Badge variant="danger">tenant {selected.status}</Badge>
                      )}
                    </div>
                    <span className="text-caption text-muted">
                      Period ends {formatDate(sub.period_end)}
                    </span>
                  </div>
                  <div className="mt-8">
                    <Meter
                      size="hero"
                      label="Allowance used"
                      usedNano={sub.used_nano}
                      includedNano={sub.included_nano}
                      periodEnd={sub.period_end}
                    />
                  </div>
                </>
              ) : (
                <EmptyState
                  icon={Receipt}
                  title="No subscription on this tenant"
                  body="Usage is billed straight to prepaid credit. Assign a plan below to grant a monthly allowance."
                />
              )}
            </Card>

            <Card>
              <p className="text-label text-muted">Credit balance</p>
              <p className="mt-6">
                <Money
                  nano={account.data?.balance_nano || 0}
                  size="hero"
                  sign="none"
                  tone={(Number(account.data?.balance_nano) || 0) <= 0 ? 'danger' : undefined}
                />
              </p>
              <div className="mt-6">
                <DescriptionList
                  items={[
                    { label: 'Tenant', value: tenantId, mono: true },
                    ...(selected
                      ? [
                          { label: 'Name', value: selected.name || '—' },
                          { label: 'Status', value: selected.status || '—' },
                        ]
                      : []),
                  ]}
                />
              </div>
            </Card>
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 lg:gap-5">
            <TopUpPanel tenantId={tenantId} onDone={refreshAll} toast={toast} />
            <SubscriptionPanel
              tenantId={tenantId}
              tiers={tierList}
              current={sub}
              onDone={refreshAll}
              toast={toast}
            />
          </div>

          <Card title="Credit ledger" description="Newest 200 movements on this tenant.">
            {ledger.loading ? (
              <div className="space-y-2">
                {Array.from({ length: 6 }, (_, i) => (
                  <Skeleton key={i} className="h-11 w-full" />
                ))}
              </div>
            ) : ledger.error ? (
              <ErrorState
                title="Could not load the ledger"
                detail={apiErrorMessage(ledger.error)}
                onRetry={ledger.reload}
              />
            ) : !Array.isArray(ledger.data) || ledger.data.length === 0 ? (
              <EmptyState
                icon={Receipt}
                title="No ledger entries"
                body="Nothing has been charged or credited on this tenant yet. Add credit to open the account."
              />
            ) : (
              <LedgerTable entries={ledger.data.slice(0, 50)} />
            )}
          </Card>
        </>
      )}
    </div>
  );
}

/* ── Move money ──────────────────────────────────────────────────── */

function TopUpPanel({ tenantId, onDone, toast }) {
  const [amount, setAmount] = useState('50');
  const [kind, setKind] = useState('topup');
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);

  const parsed = Number(amount);
  const valid = Number.isFinite(parsed) && parsed !== 0;

  async function submit(e) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true);
    try {
      const res = await client.post('/api/billing/topup', {
        tenant_id: tenantId,
        amount_usd: parsed,
        kind,
        note,
      });
      toast(`Balance is now ${formatUSD(res.data.balance_usd)}`);
      setNote('');
      onDone();
    } catch (err) {
      toast.error(apiErrorMessage(err, 'Could not apply the credit.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card title="Move credit" description="Writes an append-only ledger entry. Only owners may do this.">
      <form className="space-y-4" onSubmit={submit}>
        <div className="grid grid-cols-2 gap-4">
          <Input
            label="Amount (USD)"
            type="number"
            step="0.01"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            hint="Negative values claw credit back."
          />
          <Select label="Kind" value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="topup">Top-up</option>
            <option value="refund">Refund</option>
            <option value="adjustment">Adjustment</option>
          </Select>
        </div>
        <Input
          label="Note"
          placeholder="Invoice number, ticket, or reason"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
        <div className="flex justify-end">
          <Button type="submit" variant="primary" size="md" loading={busy} disabled={!valid}>
            <CreditCard className="h-4 w-4" aria-hidden="true" />
            Apply {valid ? formatUSD(parsed) : ''}
          </Button>
        </div>
      </form>
    </Card>
  );
}

function SubscriptionPanel({ tenantId, tiers, current, onDone, toast }) {
  const [tier, setTier] = useState('');
  const [included, setIncluded] = useState('');
  const [days, setDays] = useState('30');
  const [overage, setOverage] = useState(true);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setTier(current?.tier_id || '');
    setIncluded(current ? String((Number(current.included_nano) || 0) / 1e9) : '');
    setOverage(current ? current.overage_enabled !== false : true);
  }, [current, tenantId]);

  const includedNum = Number(included);
  const valid = tier && Number.isFinite(includedNum) && includedNum >= 0;

  async function submit(e) {
    e.preventDefault();
    if (!valid) return;
    if (
      current &&
      !window.confirm(
        'Saving starts a new billing period and resets used allowance to zero. Continue?'
      )
    ) {
      return;
    }
    setBusy(true);
    try {
      await client.post('/api/billing/subscription', {
        tenant_id: tenantId,
        tier_id: tier,
        included_usd: includedNum,
        period_days: Number(days) || 30,
        overage_enabled: overage,
      });
      toast('Subscription saved — a new period has started');
      onDone();
    } catch (err) {
      toast.error(apiErrorMessage(err, 'Could not save the subscription.'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card
      title="Plan"
      description="Assigning a plan opens a new period and resets the allowance counter."
    >
      <form className="space-y-4" onSubmit={submit}>
        <Select label="Tier" value={tier} onChange={(e) => setTier(e.target.value)}>
          <option value="">Select a tier…</option>
          {tiers.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name || t.id}
              {t.monthly_price_cents
                ? ` — ${formatUSD((t.monthly_price_cents || 0) / 100)}/mo`
                : ''}
            </option>
          ))}
        </Select>
        <div className="grid grid-cols-2 gap-4">
          <Input
            label="Included allowance (USD)"
            type="number"
            min="0"
            step="0.01"
            value={included}
            onChange={(e) => setIncluded(e.target.value)}
          />
          <Input
            label="Period length (days)"
            type="number"
            min="1"
            step="1"
            value={days}
            onChange={(e) => setDays(e.target.value)}
          />
        </div>
        <Select
          label="Overage"
          value={overage ? 'on' : 'off'}
          onChange={(e) => setOverage(e.target.value === 'on')}
        >
          <option value="on">Allowed — billed to credit</option>
          <option value="off">Blocked at the allowance</option>
        </Select>
        <div className="flex justify-end">
          <Button type="submit" variant="primary" size="md" loading={busy} disabled={!valid}>
            {current ? 'Start new period' : 'Assign plan'}
          </Button>
        </div>
      </form>
    </Card>
  );
}
