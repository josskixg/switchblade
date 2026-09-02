import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Download, ScrollText } from 'lucide-react';
import { client } from '../../api/client';
import Card from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import Select from '../../components/ui/Select';
import Skeleton from '../../components/ui/Skeleton';
import Pagination from '../../components/ui/Pagination';
import { useAsync, paginate } from '../shared/data';
import { LedgerTable, kindLabel } from '../shared/LedgerTable';
import {
  EmptyState, ErrorState, Money, PageHeader, apiErrorMessage, formatDateTime,
} from '../shared/ui';

const PER_PAGE = 25;

export default function Ledger() {
  const navigate = useNavigate();
  const [kind, setKind] = useState('all');
  const [page, setPage] = useState(0);

  const { data, loading, error, reload } = useAsync(
    () => client.get('/api/billing/ledger').then((r) => r.data),
    []
  );

  const entries = useMemo(() => (Array.isArray(data) ? data : []), [data]);
  const kinds = useMemo(
    () => Array.from(new Set(entries.map((e) => e.kind))).sort(),
    [entries]
  );
  const filtered = useMemo(
    () => (kind === 'all' ? entries : entries.filter((e) => e.kind === kind)),
    [entries, kind]
  );
  const { slice, totalPages, page: safePage } = paginate(filtered, page, PER_PAGE);

  const netNano = filtered.reduce((sum, e) => sum + (Number(e.amount_nano) || 0), 0);

  function exportCsv() {
    const header = 'created_at,kind,model,request_id,note,amount_usd,balance_after_usd\n';
    const rows = filtered
      .map((e) =>
        [
          formatDateTime(e.created_at),
          e.kind,
          e.model || '',
          e.request_id || '',
          (e.note || '').replace(/[",\n]/g, ' '),
          e.amount_usd,
          e.balance_after_usd,
        ]
          .map((v) => `"${String(v ?? '')}"`)
          .join(',')
      )
      .join('\n');
    const blob = new Blob([header + rows], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'switchblade-ledger.csv';
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-8">
      <PageHeader
        title="Credit ledger"
        breadcrumb={[{ label: 'Billing' }, { label: 'Ledger' }]}
        description="Append-only record of every charge and credit on this account."
        actions={
          <Button
            type="button"
            variant="secondary"
            size="md"
            onClick={exportCsv}
            disabled={filtered.length === 0}
          >
            <Download className="h-4 w-4" aria-hidden="true" />
            Export CSV
          </Button>
        }
      />

      <Card>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <Select
            aria-label="Filter by kind"
            value={kind}
            onChange={(e) => {
              setKind(e.target.value);
              setPage(0);
            }}
            wrapperClassName="w-48"
          >
            <option value="all">All entries</option>
            {kinds.map((k) => (
              <option key={k} value={k}>
                {kindLabel(k)}
              </option>
            ))}
          </Select>

          {!loading && !error && filtered.length > 0 && (
            <p className="text-label text-muted">
              {filtered.length} {filtered.length === 1 ? 'entry' : 'entries'} · net{' '}
              <Money nano={netNano} className="text-text" />
            </p>
          )}
        </div>

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 8 }, (_, i) => (
              <Skeleton key={i} className="h-11 w-full" />
            ))}
          </div>
        ) : error ? (
          <ErrorState
            title="Could not load the ledger"
            detail={apiErrorMessage(error)}
            onRetry={reload}
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={ScrollText}
            title={kind === 'all' ? 'Nothing billed yet' : `No ${kindLabel(kind).toLowerCase()} entries`}
            body={
              kind === 'all'
                ? 'Charges appear the moment your first request is metered. Create a key to get started.'
                : 'Try a different entry kind.'
            }
            action={
              kind === 'all'
                ? { label: 'Create an API key', onClick: () => navigate('/app/keys') }
                : { label: 'Show all entries', onClick: () => setKind('all') }
            }
          />
        ) : (
          <>
            <LedgerTable entries={slice} />
            <Pagination currentPage={safePage} totalPages={totalPages} onPageChange={setPage} />
            {entries.length >= 200 && (
              <p className="mt-4 text-caption text-muted">
                Showing the 200 most recent entries. Export the CSV for a full period record.
              </p>
            )}
          </>
        )}
      </Card>
    </div>
  );
}
