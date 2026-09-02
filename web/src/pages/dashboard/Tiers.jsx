import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Skeleton from '../../components/ui/Skeleton';
import QuotaBar from '../../components/ui/QuotaBar';
import TierBadge from '../../components/ui/TierBadge';
import { Users, Activity } from 'lucide-react';

export default function Tiers() {
  const [quotaStatus, setQuotaStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const toast = useToast();

  useEffect(() => {
    fetchQuotaStatus();
  }, []);

  async function fetchQuotaStatus() {
    try {
      const res = await client.get('/api/v1/tiers/quota-status');
      setQuotaStatus(res.data);
    } catch {
      toast.error('Failed to load quota status');
    } finally {
      setLoading(false);
    }
  }

  if (loading) {
    return (
      <div className="space-y-6">
        <div>
          <Skeleton className="h-8 w-48 mb-2" />
          <Skeleton className="h-4 w-72" />
        </div>
      <div className="grid gap-4 grid-cols-1 md:grid-cols-3">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      </div>
    );
  }

  const tiers = quotaStatus?.tiers || [];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">Tiers</h1>
        <p className="text-sm text-muted mt-1">Account quota and usage per tier</p>
      </div>

        <div className="grid gap-4 grid-cols-1 md:grid-cols-3">
        {tiers.length === 0 ? (
          <div className="col-span-3 text-center py-12 text-muted text-sm">
            No tier data available
          </div>
        ) : (
          (tiers || []).map((tier) => (
            <Card key={tier.tier}>
              <div className="flex items-center justify-between mb-4">
                <TierBadge tier={tier.tier} />
                <div className="flex items-center gap-1.5 text-xs text-muted">
                  <Users className="w-3.5 h-3.5" />
                  <span className="tabular-nums">{tier.total_accounts}</span>
                </div>
              </div>

              <div className="space-y-4">
                <div>
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-xs text-muted">Active</span>
                    <span className="text-sm font-semibold text-text tabular-nums">
                      {tier.active_accounts}
                    </span>
                  </div>
                </div>

                <QuotaBar
                  tier={tier.tier}
                  used={tier.quota_used}
                  total={tier.quota_total}
                  label="Quota Used"
                />

                <div className="pt-2 border-t border-border">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted">Remaining</span>
                    <span className="text-sm font-mono text-text tabular-nums">
                      {Math.max(0, tier.quota_total - tier.quota_used).toLocaleString()}
                    </span>
                  </div>
                </div>
              </div>
            </Card>
          ))
        )}
      </div>

      {quotaStatus?.summary && (
        <Card>
          <h3 className="text-base font-semibold text-text mb-4">Summary</h3>
          <div className="grid gap-4 sm:grid-cols-3">
            <div>
              <p className="text-xs text-muted uppercase tracking-wider mb-1">Total Accounts</p>
              <p className="text-2xl font-bold text-text tabular-nums">
                {quotaStatus.summary.total_accounts}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted uppercase tracking-wider mb-1">Active Accounts</p>
              <p className="text-2xl font-bold text-text tabular-nums flex items-center gap-2">
                <Activity className="w-5 h-5 text-success" />
                {quotaStatus.summary.active_accounts}
              </p>
            </div>
            <div>
              <p className="text-xs text-muted uppercase tracking-wider mb-1">Total Quota</p>
              <p className="text-2xl font-bold text-text tabular-nums">
                {quotaStatus.summary.total_quota.toLocaleString()}
              </p>
            </div>
          </div>
        </Card>
      )}
    </div>
  );
}
