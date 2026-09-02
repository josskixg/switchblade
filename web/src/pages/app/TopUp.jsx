import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Check, ShieldAlert } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import { useToast } from '../../components/ui/Toast';
import { useAuthStore } from '../../store/auth';
import { useBillingStore } from '../shared/billingStore';
import {
  DescriptionList, EmptyState, Money, PageHeader, apiErrorMessage, formatUSD,
} from '../shared/ui';

const PRESETS = [10, 25, 50, 100, 250];

export default function TopUp() {
  const toast = useToast();
  const navigate = useNavigate();
  const tenant = useAuthStore((s) => s.tenant);
  const account = useBillingStore((s) => s.account);
  const reloadBilling = useBillingStore((s) => s.load);

  const tenantId = account?.tenant_id || tenant?.id || '';
  const [amount, setAmount] = useState('25');
  const [note, setNote] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [denied, setDenied] = useState(false);

  const parsed = Number(amount);
  const valid = Number.isFinite(parsed) && parsed > 0;
  // The preview stays in integer nanodollars (whole cents × 1e7) so it cannot
  // drift from the balance the server will report after the ledger write.
  const projected =
    (Number(account?.balance_nano) || 0) + (valid ? Math.round(parsed * 100) * 10_000_000 : 0);

  async function submit(e) {
    e.preventDefault();
    if (!valid || !tenantId) return;
    setSubmitting(true);
    try {
      const res = await client.post('/api/billing/topup', {
        tenant_id: tenantId,
        amount_usd: parsed,
        kind: 'topup',
        note: note || 'Self-service top-up',
      });
      toast(`Added ${formatUSD(parsed)} — balance is now ${formatUSD(res.data.balance_usd)}`);
      await reloadBilling({ force: true });
      navigate('/app/billing');
    } catch (err) {
      if (err.response?.status === 403) {
        setDenied(true);
      } else {
        toast.error(apiErrorMessage(err, 'Could not add credit.'));
      }
    } finally {
      setSubmitting(false);
    }
  }

  if (denied) {
    return (
      <div className="space-y-8">
        <PageHeader
          title="Add credit"
          breadcrumb={[{ label: 'Billing' }, { label: 'Add credit' }]}
        />
        <Card>
          <EmptyState
            icon={ShieldAlert}
            title="Your role cannot add credit"
            body="Only an account owner can move money on this tenant. Ask your owner to top up, or sign in with an owner account."
            action={{ label: 'Back to billing', onClick: () => navigate('/app/billing') }}
          />
        </Card>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title="Add credit"
        breadcrumb={[{ label: 'Billing' }, { label: 'Add credit' }]}
        description="Credit covers overage and any usage outside your plan allowance."
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-5">
        <Card className="lg:col-span-2">
          <form onSubmit={submit} className="space-y-4">
            <div>
              <p className="mb-1.5 text-label text-text">Amount</p>
              <div className="flex flex-wrap gap-2">
                {PRESETS.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => setAmount(String(p))}
                    aria-pressed={amount === String(p)}
                    className={`rounded-[var(--r-sm,6px)] border px-3 py-1.5 text-label tabular-nums transition-[background-color,border-color,color] duration-150 ${
                      amount === String(p)
                        ? 'border-primary bg-primary/[0.08] text-primary dark:bg-primary/[0.12]'
                        : 'border-border-strong text-text-secondary hover:bg-surface-hover hover:text-text'
                    }`}
                  >
                    {formatUSD(p)}
                  </button>
                ))}
              </div>
            </div>

            <Input
              label="Custom amount (USD)"
              type="number"
              min="1"
              step="0.01"
              inputMode="decimal"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              error={amount !== '' && !valid ? 'Enter an amount greater than zero.' : undefined}
              hint="Credit never expires and is spent only after your plan allowance."
            />

            <Input
              label="Note (optional)"
              placeholder="Purchase order, reference, or reason"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />

            <div className="flex items-center justify-end gap-2 pt-2">
              <Link to="/app/billing">
                <Button type="button" variant="ghost" size="md">
                  Cancel
                </Button>
              </Link>
              <Button
                type="submit"
                variant="primary"
                size="md"
                loading={submitting}
                disabled={!valid || !tenantId}
              >
                <Check className="h-4 w-4" aria-hidden="true" />
                Add {valid ? formatUSD(parsed) : 'credit'}
              </Button>
            </div>

            {!tenantId && (
              <p className="text-caption text-danger">
                No tenant is attached to this session, so credit cannot be applied. Sign in again.
              </p>
            )}
          </form>
        </Card>

        <Card title="After this top-up">
          <DescriptionList
            items={[
              {
                label: 'Current balance',
                value: <Money nano={account?.balance_nano || 0} sign="none" />,
              },
              {
                label: 'Adding',
                value: valid ? <Money usd={parsed} sign="always" /> : '—',
              },
              {
                label: 'New balance',
                value: <Money nano={projected} sign="none" className="text-text" />,
              },
              { label: 'Account', value: tenantId || '—', mono: true },
            ]}
          />
          <p className="mt-4 text-caption text-muted">
            Credit is recorded as an append-only ledger entry and is spent only after your plan
            allowance is exhausted.
          </p>
        </Card>
      </div>
    </div>
  );
}
