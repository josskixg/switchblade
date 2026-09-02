import { useMemo } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Activity, Cable, Server, Users } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import Skeleton from '../../components/ui/Skeleton';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '../../components/ui/Table';
import { useAsync } from '../shared/data';
import {
  EmptyState, ErrorState, JackField, PageHeader, StatTile,
  apiErrorMessage, formatCompact, formatDateTime,
} from '../shared/ui';

/** Maps an accounts row onto a JackField port state. */
export function portState(a) {
  if (!a.enabled) return 'disabled';
  if (a.status === 'error') return 'error';
  if (a.status !== 'active') return 'cooldown';
  if (Number(a.quota_limit) > 0 && Number(a.quota_remaining) <= 0) return 'cooldown';
  return 'active';
}

function maskEmail(email) {
  if (!email) return '';
  const [name, domain] = String(email).split('@');
  if (!domain) return `${name.slice(0, 2)}***`;
  return `${name.slice(0, 2)}***@${domain}`;
}

export default function Fleet() {
  const navigate = useNavigate();
  const stats = useAsync(() => client.get('/api/stats').then((r) => r.data), []);
  const accounts = useAsync(() => client.get('/api/accounts').then((r) => r.data), []);
  const tenants = useAsync(() => client.get('/api/tenants').then((r) => r.data), []);

  const rows = useMemo(() => (Array.isArray(stats.data) ? stats.data : []), [stats.data]);
  const accountList = useMemo(
    () => (Array.isArray(accounts.data) ? accounts.data : []),
    [accounts.data]
  );
  const tenantList = useMemo(
    () => (Array.isArray(tenants.data) ? tenants.data : []),
    [tenants.data]
  );

  const ports = useMemo(
    () =>
      accountList.map((a) => ({
        id: a.id,
        provider: a.provider,
        state: portState(a),
        label: maskEmail(a.email),
        lastUsed: a.last_used_at ? formatDateTime(a.last_used_at) : 'never',
        quotaRemaining: Number(a.quota_limit) > 0 ? a.quota_remaining : null,
      })),
    [accountList]
  );

  const byProvider = useMemo(() => {
    const map = new Map();
    for (const r of rows) {
      const key = r.provider || 'unknown';
      const cur = map.get(key) || { provider: key, requests: 0, tokens: 0, models: new Set() };
      cur.requests += Number(r.request_count) || 0;
      cur.tokens += Number(r.total_tokens) || 0;
      if (r.model) cur.models.add(r.model);
      map.set(key, cur);
    }
    return Array.from(map.values()).sort((a, b) => b.requests - a.requests);
  }, [rows]);

  const totalRequests = byProvider.reduce((s, p) => s + p.requests, 0);
  const totalTokens = byProvider.reduce((s, p) => s + p.tokens, 0);
  const lit = ports.filter((p) => p.state === 'active').length;

  return (
    <div className="space-y-8">
      <PageHeader
        title="Fleet"
        description="Pool health, provider mix, and platform throughput."
        actions={
          <Button
            type="button"
            variant="secondary"
            size="md"
            onClick={() => {
              stats.reload();
              accounts.reload();
              tenants.reload();
            }}
          >
            Refresh
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
        <StatTile
          label="Accounts lit"
          value={lit}
          unit={`of ${ports.length}`}
          icon={Cable}
          loading={accounts.loading}
          empty={Boolean(accounts.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Tenants"
          value={tenantList.length}
          unit={`${tenantList.filter((t) => t.status === 'active').length} active`}
          icon={Users}
          loading={tenants.loading}
          empty={Boolean(tenants.error)}
          emptyLabel="Unavailable"
        />
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
          icon={Server}
          loading={stats.loading}
          empty={Boolean(stats.error)}
          emptyLabel="Unavailable"
        />
      </div>

      {/* ── The patch bay ─────────────────────────────────────────── */}
      <Card
        title="Account pool"
        description="One port per pooled provider account, lit by health."
        headerAction={
          <Link to="/admin/providers/accounts">
            <Button type="button" variant="ghost" size="sm">
              Manage accounts
            </Button>
          </Link>
        }
      >
        {accounts.loading ? (
          <Skeleton className="h-24 w-full" />
        ) : accounts.error ? (
          <ErrorState
            title="Could not read the account pool"
            detail={apiErrorMessage(accounts.error)}
            onRetry={accounts.reload}
          />
        ) : ports.length === 0 ? (
          <EmptyState
            icon={Cable}
            title="No accounts in the pool"
            body="The gateway cannot route anything until at least one provider account is added."
            action={{
              label: 'Add an account',
              onClick: () => navigate('/admin/providers/accounts'),
            }}
          />
        ) : (
          <JackField ports={ports} />
        )}
      </Card>

      {/* ── Provider mix ──────────────────────────────────────────── */}
      <Card title="Provider mix" description="Recorded usage by upstream provider.">
        {stats.loading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }, (_, i) => (
              <Skeleton key={i} className="h-11 w-full" />
            ))}
          </div>
        ) : stats.error ? (
          <ErrorState
            title="Could not load usage"
            detail={apiErrorMessage(stats.error)}
            onRetry={stats.reload}
          />
        ) : byProvider.length === 0 ? (
          <EmptyState
            icon={Activity}
            title="No traffic recorded yet"
            body="Provider rollups appear as soon as requests are metered through the gateway."
            action={{ label: 'Configure a provider', onClick: () => navigate('/admin/providers') }}
          />
        ) : (
          <Table>
            <TableHead>
              <TableRow>
                <TableHeader>Provider</TableHeader>
                <TableHeader align="numeric">Requests</TableHeader>
                <TableHeader align="numeric">Tokens</TableHeader>
                <TableHeader align="numeric">Models</TableHeader>
                <TableHeader align="numeric">Share</TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              {byProvider.map((p) => (
                <TableRow key={p.provider}>
                  <TableCell className="font-mono text-code text-text">{p.provider}</TableCell>
                  <TableCell align="numeric">{p.requests}</TableCell>
                  <TableCell align="numeric" className="text-text-secondary">
                    {formatCompact(p.tokens)}
                  </TableCell>
                  <TableCell align="numeric" className="text-text-secondary">
                    {p.models.size}
                  </TableCell>
                  <TableCell align="numeric" className="text-muted">
                    {totalRequests > 0
                      ? `${Math.round((p.requests / totalRequests) * 100)}%`
                      : '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </div>
  );
}
