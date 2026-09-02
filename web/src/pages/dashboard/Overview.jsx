import { useEffect, useMemo, useState } from 'react';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { Activity, AlertTriangle, Inbox, Key, Zap } from 'lucide-react';
import { client } from '../../api/client';
import { useAuthStore } from '../../store/auth';
import Card from '../../components/ui/Card';
import ChartFrame from '../../components/ui/ChartFrame';
import DataTable from '../../components/ui/DataTable';
import EmptyState from '../../components/ui/EmptyState';
import PageHeader from '../../components/ui/PageHeader';
import StatTile from '../../components/ui/StatTile';
import { Badge } from '../../components/ui/Badge';
import { tooltipProps } from '../../theme/chartTheme';
import { useAsync } from '../shared/data';
import { apiErrorMessage, formatCompact, formatDateTime } from '../shared/ui';

/*
 * Every number here is read off an endpoint. /api/stats is the usage_summary
 * rollup and returns an ARRAY of {provider, model, request_count, …} — the
 * previous version of this page read `total_requests`, `tokens_used`,
 * `active_keys` and `error_rate` off it, none of which that shape has, so all
 * four tiles printed 0 whatever the traffic.
 *
 * There is no time-series endpoint: usage_summary buckets hourly but
 * db.GetUsageStats groups the bucket away. Until one exists this page shows the
 * model mix it can actually prove, not a 24h curve it would have to invent.
 */

const LOG_SAMPLE = 200;

export default function Overview() {
  const token = useAuthStore((s) => s.token);
  const [liveEvents, setLiveEvents] = useState([]);

  const stats = useAsync(() => client.get('/api/stats').then((r) => r.data), []);
  const logs = useAsync(
    () => client.get('/api/logs', { params: { limit: LOG_SAMPLE } }).then((r) => r.data),
    []
  );
  const keys = useAsync(() => client.get('/api/keys').then((r) => r.data), []);

  useEffect(() => {
    if (!token) return undefined;
    const source = new EventSource(`/api/events?token=${encodeURIComponent(token)}`);

    const push = (type) => (e) => {
      try {
        setLiveEvents((prev) => [{ type, data: JSON.parse(e.data), ts: Date.now() }, ...prev].slice(0, 20));
      } catch {
        /* a malformed frame is not worth tearing the stream down for */
      }
    };

    source.addEventListener('request_complete', push('request'));
    source.addEventListener('request_error', push('error'));
    source.addEventListener('tenant_blocked', push('blocked'));
    source.addEventListener('tenant_unblocked', push('unblocked'));

    return () => source.close();
  }, [token]);

  const rows = useMemo(() => (Array.isArray(stats.data) ? stats.data : []), [stats.data]);
  const logRows = useMemo(() => (Array.isArray(logs.data) ? logs.data : []), [logs.data]);
  const keyRows = useMemo(() => (Array.isArray(keys.data) ? keys.data : []), [keys.data]);

  const totalRequests = rows.reduce((s, r) => s + (Number(r.request_count) || 0), 0);
  const totalTokens = rows.reduce((s, r) => s + (Number(r.total_tokens) || 0), 0);
  const activeKeys = keyRows.filter((k) => k.enabled).length;

  const failed = logRows.filter((l) => l.status && l.status !== 'success').length;
  const errorRate = logRows.length > 0 ? (failed / logRows.length) * 100 : null;

  const byModel = useMemo(() => {
    const map = new Map();
    for (const r of rows) {
      const key = r.model || 'unknown';
      map.set(key, (map.get(key) || 0) + (Number(r.request_count) || 0));
    }
    return Array.from(map, ([model, requests]) => ({ model, requests }))
      .sort((a, b) => b.requests - a.requests)
      .slice(0, 8);
  }, [rows]);

  const chartState = stats.loading ? 'loading' : stats.error ? 'error' : byModel.length === 0 ? 'empty' : 'ready';

  return (
    <div className="space-y-8">
      <PageHeader title="Dashboard" description="Recorded throughput across the gateway." />

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
          icon={Zap}
          loading={stats.loading}
          empty={Boolean(stats.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Active keys"
          value={activeKeys}
          unit={keyRows.length ? `of ${keyRows.length}` : undefined}
          icon={Key}
          loading={keys.loading}
          empty={Boolean(keys.error)}
          emptyLabel="Unavailable"
        />
        <StatTile
          label="Error rate"
          value={errorRate == null ? null : `${errorRate.toFixed(1)}%`}
          unit={logRows.length ? `of last ${logRows.length}` : undefined}
          icon={AlertTriangle}
          deltaDirection="down-is-good"
          loading={logs.loading}
          empty={Boolean(logs.error) || errorRate == null}
          emptyLabel={logs.error ? 'Unavailable' : 'No requests yet'}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-5">
        <ChartFrame
          className="lg:col-span-2"
          title="Requests by model"
          description="From the usage_summary rollup."
          state={chartState}
          error={{ title: 'Could not load the rollup', detail: apiErrorMessage(stats.error) }}
          onRetry={stats.reload}
          empty={{
            icon: Activity,
            title: 'No traffic recorded yet',
            body: 'Model rollups appear as soon as requests are metered through the gateway.',
          }}
          height={240}
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
                <Bar dataKey="requests" name="Requests" fill={theme.series[0]} radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </ChartFrame>

        <Card title="Live activity" description="Streaming from /api/events.">
          {liveEvents.length === 0 ? (
            <EmptyState
              icon={Inbox}
              title="Nothing yet"
              body="Events appear here the moment a request completes."
            />
          ) : (
            <ul className="space-y-2">
              {liveEvents.slice(0, 8).map((e) => (
                <li key={e.ts + e.type} className="flex items-center justify-between gap-3">
                  <span className="truncate font-mono text-code text-text-secondary">
                    {e.data?.model || e.data?.tenant_id || e.type}
                  </span>
                  <Badge variant={e.type === 'request' ? 'success' : e.type === 'error' ? 'danger' : 'default'}>
                    {e.type}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <DataTable
        state={logs.loading ? 'loading' : logs.error ? 'error' : 'ready'}
        error={{ title: 'Could not load recent requests', detail: apiErrorMessage(logs.error) }}
        onRetry={logs.reload}
        empty={{ icon: Inbox, title: 'No requests logged yet' }}
        rows={logRows.slice(0, 8)}
        columns={[
          {
            key: 'created_at',
            header: 'Time',
            render: (l) => <span className="whitespace-nowrap text-caption text-muted">{formatDateTime(l.created_at)}</span>,
          },
          { key: 'provider', header: 'Provider' },
          {
            key: 'model',
            header: 'Model',
            render: (l) => <span className="font-mono text-code text-text">{l.model || '—'}</span>,
          },
          {
            key: 'status',
            header: 'Status',
            render: (l) => (
              <Badge variant={l.status === 'success' ? 'success' : 'default'}>{l.status || '—'}</Badge>
            ),
          },
          {
            key: 'total_tokens',
            header: 'Tokens',
            align: 'numeric',
            render: (l) => formatCompact(l.total_tokens),
          },
        ]}
      />
    </div>
  );
}
