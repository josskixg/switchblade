import { useMemo, useState } from 'react';
import { Layers, Search } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Skeleton from '../../components/ui/Skeleton';
import { Badge } from '../../components/ui/Badge';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '../../components/ui/Table';
import { useAsync } from '../shared/data';
import {
  EmptyState, ErrorState, Money, PageHeader, apiErrorMessage,
} from '../shared/ui';

/** What the customer actually pays: list rate with the operator's margin applied. */
export function effectiveNano(baseNano, marginBps) {
  const base = Number(baseNano) || 0;
  const bps = Number(marginBps) || 0;
  return Math.round((base * (10000 + bps)) / 10000);
}

export default function Models() {
  const [query, setQuery] = useState('');
  const { data, loading, error, reload } = useAsync(
    () => client.get('/api/billing/pricing').then((r) => r.data),
    []
  );

  const rows = useMemo(() => {
    const list = Array.isArray(data) ? data.filter((p) => p.enabled) : [];
    const q = query.trim().toLowerCase();
    return q ? list.filter((p) => p.model.toLowerCase().includes(q)) : list;
  }, [data, query]);

  return (
    <div className="space-y-8">
      <PageHeader
        title="Models & prices"
        description="Every model this gateway serves, with the rate you are billed per million tokens."
      />

      <Card>
        <div className="mb-4 max-w-sm">
          <Input
            aria-label="Filter models"
            placeholder="Filter by model name"
            icon={<Search className="h-4 w-4" aria-hidden="true" />}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-11 w-full" />
            ))}
          </div>
        ) : error ? (
          <ErrorState
            title="Could not load the rate card"
            detail={apiErrorMessage(error)}
            onRetry={reload}
          />
        ) : rows.length === 0 ? (
          <EmptyState
            icon={Layers}
            title={query ? `No model matches “${query}”` : 'No models are priced yet'}
            body={
              query
                ? 'Try a shorter search — model ids are versioned, so "gpt-4o" matches more than "gpt-4o-2024".'
                : 'Your operator has not published a rate card. Requests may still route, but no price is shown here.'
            }
            action={query ? { label: 'Clear filter', onClick: () => setQuery('') } : undefined}
          />
        ) : (
          <>
            {/* Versioned model ids overflow a phone, so rows stack below `sm`. */}
            <ul className="divide-y divide-border-subtle sm:hidden">
              {rows.map((p) => (
                <li key={p.model} className="py-3">
                  <p className="truncate font-mono text-code text-text">{p.model}</p>
                  <p className="mt-1 text-body text-text-secondary">
                    <Money nano={effectiveNano(p.input_nano_per_mtok, p.margin_bps)} sign="none" />
                    <span className="text-muted"> in · </span>
                    <Money nano={effectiveNano(p.output_nano_per_mtok, p.margin_bps)} sign="none" />
                    <span className="text-muted"> out per 1M tokens</span>
                  </p>
                </li>
              ))}
            </ul>

            <div className="hidden sm:block">
              <Table>
                <TableHead>
                  <TableRow>
                    <TableHeader>Model</TableHeader>
                    <TableHeader align="numeric">Input / 1M tokens</TableHeader>
                    <TableHeader align="numeric">Output / 1M tokens</TableHeader>
                    <TableHeader>Availability</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {rows.map((p) => (
                    <TableRow key={p.model}>
                      <TableCell className="font-mono text-code text-text">{p.model}</TableCell>
                      <TableCell align="numeric">
                        <Money nano={effectiveNano(p.input_nano_per_mtok, p.margin_bps)} sign="none" />
                      </TableCell>
                      <TableCell align="numeric">
                        <Money nano={effectiveNano(p.output_nano_per_mtok, p.margin_bps)} sign="none" />
                      </TableCell>
                      <TableCell>
                        <Badge variant="success">Available</Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </>
        )}
      </Card>

      <p className="text-caption text-muted">
        Prices are matched by longest prefix, so a dated snapshot such as
        <code className="mx-1 font-mono text-code text-text-secondary">gpt-4o-mini-2024-07-18</code>
        bills at the <code className="mx-1 font-mono text-code text-text-secondary">gpt-4o-mini</code>
        rate.
      </p>
    </div>
  );
}
