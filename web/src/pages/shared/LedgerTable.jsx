import { Badge } from '../../components/ui/Badge';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '../../components/ui/Table';
import { Money, formatDateTime } from './ui';

const KIND_LABEL = {
  topup: 'Top-up',
  usage: 'Usage',
  subscription: 'Allowance',
  refund: 'Refund',
  adjustment: 'Adjustment',
};

const KIND_VARIANT = {
  topup: 'success',
  refund: 'success',
  subscription: 'primary',
  usage: 'default',
  adjustment: 'warning',
};

export function kindLabel(kind) {
  return KIND_LABEL[kind] || kind;
}

/* Six columns cannot fit a 375px phone, so below `sm` each entry renders as a
 * stacked row instead of forcing a sideways scroll through money figures. */
export function LedgerTable({ entries, showBalance = true, showRequest = true }) {
  return (
    <>
      <ul className="divide-y divide-border-subtle sm:hidden">
        {entries.map((e) => (
          <li key={e.id} className="flex items-start justify-between gap-3 py-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={KIND_VARIANT[e.kind] || 'default'}>{kindLabel(e.kind)}</Badge>
                <span className="text-caption text-muted">{formatDateTime(e.created_at)}</span>
              </div>
              <p className="mt-1 truncate">
                {e.model ? (
                  <span className="font-mono text-code text-text">{e.model}</span>
                ) : (
                  <span className="text-body text-text-secondary">{e.note || '—'}</span>
                )}
              </p>
              {showRequest && e.request_id && (
                <p className="mt-0.5 truncate font-mono text-code text-muted">{e.request_id}</p>
              )}
            </div>
            <div className="shrink-0 text-right">
              <Money nano={e.amount_nano} sign="auto" />
              {showBalance && (
                <p className="mt-0.5 text-caption text-muted">
                  <Money nano={e.balance_after_nano} sign="none" size="label" className="text-muted" />{' '}
                  after
                </p>
              )}
            </div>
          </li>
        ))}
      </ul>

      <div className="hidden sm:block">
        <Table>
          <TableHead>
            <TableRow>
              <TableHeader>Date</TableHeader>
              <TableHeader>Kind</TableHeader>
              <TableHeader>Detail</TableHeader>
              {showRequest && <TableHeader>Request</TableHeader>}
              <TableHeader align="numeric">Amount</TableHeader>
              {showBalance && <TableHeader align="numeric">Balance after</TableHeader>}
            </TableRow>
          </TableHead>
          <TableBody>
            {entries.map((e) => (
              <TableRow key={e.id}>
                <TableCell className="whitespace-nowrap text-caption text-muted">
                  {formatDateTime(e.created_at)}
                </TableCell>
                <TableCell>
                  <Badge variant={KIND_VARIANT[e.kind] || 'default'}>{kindLabel(e.kind)}</Badge>
                </TableCell>
                <TableCell className="max-w-[24ch] truncate">
                  {e.model ? (
                    <span className="font-mono text-code text-text">{e.model}</span>
                  ) : (
                    <span className="text-text-secondary">{e.note || '—'}</span>
                  )}
                </TableCell>
                {showRequest && (
                  <TableCell className="font-mono text-code text-muted">
                    {e.request_id || '—'}
                  </TableCell>
                )}
                <TableCell align="numeric">
                  <Money nano={e.amount_nano} sign="auto" />
                </TableCell>
                {showBalance && (
                  <TableCell align="numeric" className="text-text-secondary">
                    <Money nano={e.balance_after_nano} sign="none" />
                  </TableCell>
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  );
}

export default LedgerTable;
